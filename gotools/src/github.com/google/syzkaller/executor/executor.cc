// Copyright 2017 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

// +build

#include <algorithm>
#include <errno.h>
#include <limits.h>
#include <sched.h>
#include <signal.h>
#include <stdarg.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#if !GOOS_windows
#include <unistd.h>
#endif

#include "defs.h"

#include "hypercall.h"

#if defined(__GNUC__)
#define SYSCALLAPI
#define NORETURN __attribute__((noreturn))
#define PRINTF(fmt, args) __attribute__((format(printf, fmt, args)))
#else
// Assuming windows/cl.
#define SYSCALLAPI WINAPI
#define NORETURN __declspec(noreturn)
#define PRINTF(fmt, args)
#define __thread __declspec(thread)
#endif

#ifndef GIT_REVISION
#define GIT_REVISION "unknown"
#endif

#define ARRAY_SIZE(x) (sizeof(x) / sizeof((x)[0]))

enum cov_type {
	code_coverage = 0,
	read_from_coverage,
	nr_cov_type,
};

// uint64 is impossible to printf without using the clumsy and verbose "%" PRId64.
// So we define and use uint64. Note: pkg/csource does s/uint64/uint64/.
// Also define uint32/16/8 for consistency.
typedef unsigned long long uint64;
typedef unsigned int uint32;
typedef unsigned short uint16;
typedef unsigned char uint8;

// exit/_exit do not necessary work (e.g. if fuzzer sets seccomp filter that prohibits exit_group).
// Use doexit instead.  We must redefine exit to something that exists in stdlib,
// because some standard libraries contain "using ::exit;", but has different signature.
#define exit vsnprintf

// Dynamic memory allocation reduces test reproducibility across different libc versions and kernels.
// malloc will cause unspecified number of additional mmap's at unspecified locations.
// For small objects prefer stack allocations, for larger -- either global objects (this may have
// issues with concurrency), or controlled mmaps, or make the fuzzer allocate memory.
#define malloc do_not_use_malloc
#define calloc do_not_use_calloc

// Note: zircon max fd is 256.
// Some common_OS.h files know about this constant for RLIMIT_NOFILE.
const int kMaxFd = 250;
const int kMaxThreads = 4;
const int kMaxSchedule = 128;
const int kMaxVector = 32;
const int kMaxFallbackThreads = 3;
const int kMaxPendingThreads = 1;
const int kInPipeFd = kMaxFd - 1; // remapped from stdin
const int kOutPipeFd = kMaxFd - 2; // remapped from stdout
const int kCoverFd = kOutPipeFd - (kMaxThreads * kMaxFallbackThreads) * nr_cov_type;
const int kExtraCoverFd = kCoverFd - 1;
const int kMaxArgs = 9;
const int kMaxCPU = 8;
#define __mask(n) (1 << (n))
const int kCPUMask[kMaxCPU] = {
    __mask(0),
    __mask(1),
    __mask(2),
    __mask(3),
    __mask(4),
    __mask(5),
    __mask(6),
    __mask(7),
};
__attribute__((unused))
const int kCPUMaskAll = 0xff;
const int kCoverSize = 256 << 10;
const int kFailStatus = 67;

const int kSSBSwitch = 503;

// Logical error (e.g. invalid input program), use as an assert() alternative.
// If such error happens 10+ times in a row, it will be detected as a bug by syz-fuzzer.
// syz-fuzzer will fail and syz-manager will create a bug for this.
// Note: err is used for bug deduplication, thus distinction between err (constant message)
// and msg (varying part).
static NORETURN void fail(const char* err);
static NORETURN PRINTF(2, 3) void failmsg(const char* err, const char* msg, ...);
// Just exit (e.g. due to temporal ENOMEM error).
static NORETURN PRINTF(1, 2) void exitf(const char* msg, ...);
static NORETURN void doexit(int status);
#if !GOOS_fuchsia
static NORETURN void doexit_thread(int status);
#endif

// Print debug output that is visible when running syz-manager/execprog with -debug flag.
// Debug output is supposed to be relatively high-level (syscalls executed, return values, timing, etc)
// and is intended mostly for end users. If you need to debug lower-level details, use debug_verbose
// function and temporary enable it in your build by changing #if 0 below.
// This function does not add \n at the end of msg as opposed to the previous functions.
static PRINTF(1, 2) void debug_noprefix(const char* msg, ...);
static PRINTF(1, 2) void debug(const char* msg, ...);
void debug_dump_data(const char* data, int length);

#if 0
#define debug_verbose(...) debug(__VA_ARGS__)
#else
#define debug_verbose(...) (void)0
#endif

#define _DEBUG
#ifdef _DEBUG
#define WARN_ON_NOT_NULL(exp, name)                                                 \
	{                                                                           \
		unsigned long _ret = exp;                                           \
		if (_ret != 0)                                                      \
			debug("[WARN] %s returns non-zero, ret=%lu\n", name, _ret); \
	}
#else
#define WARN_ON_NOT_NULL(exp, name) \
	exp
#endif

static void receive_execute();
static void reply_execute(int status);

#if GOOS_akaros
static void resend_execute(int fd);
#endif

#if SYZ_EXECUTOR_USES_FORK_SERVER
static void receive_handshake();
static void reply_handshake();
#endif

#if SYZ_EXECUTOR_USES_SHMEM
// The output region is the only thing in executor process for which consistency matters.
// If it is corrupted ipc package will fail to parse its contents and panic.
// But fuzzer constantly invents new ways of how to corrupt the region,
// so we map the region at a (hopefully) hard to guess address with random offset,
// surrounded by unmapped pages.
// The address chosen must also work on 32-bit kernels with 1GB user address space.
const uint64 kOutputBase = 0x1b2bc20000ull;

#if SYZ_EXECUTOR_USES_FORK_SERVER
// Allocating (and forking) virtual memory for each executed process is expensive, so we only mmap
// the amount we might possibly need for the specific received prog.
const int kMaxOutputComparisons = 14 << 20; // executions with comparsions enabled are usually < 1% of all executions
// XXX: memcov consumes more memory, so we increase the the amount of
// memory.
const int kMaxOutputCoverage = 6 << 20 << 2; // coverage is needed in ~ up to 1/3 of all executions (depending on corpus rotation)
const int kMaxOutputSignal = 4 << 20 << 2;
const int kMinOutput = 256 << 10; // if we don't need to send signal, the output is rather short.
const int kInitialOutput = kMinOutput; // the minimal size to be allocated in the parent process
#else
// We don't fork and allocate the memory only once, so prepare for the worst case.
const int kInitialOutput = 14 << 20;
#endif

// TODO: allocate a smaller amount of memory in the parent once we merge the patches that enable
// prog execution with neither signal nor coverage. Likely 64kb will be enough in that case.

const int kInFd = 3;
const int kOutFd = 4;
static uint32* output_data;
static uint32* output_pos;
static int output_size;
static void mmap_output(int size);
static uint32* write_output(uint32 v);
static uint32* write_output_64(uint64 v);
static void write_completed(uint32 completed);
static uint32 hash(uint32 a);
static bool dedup(uint32 sig);
#endif // if SYZ_EXECUTOR_USES_SHMEM

uint64 start_time_ms = 0;

static bool flag_debug;
static bool flag_coverage;
static bool flag_sandbox_none;
static bool flag_sandbox_setuid;
static bool flag_sandbox_namespace;
static bool flag_sandbox_android;
static bool flag_extra_coverage;
static bool flag_net_injection;
static bool flag_net_devices;
static bool flag_net_reset;
static bool flag_cgroups;
static bool flag_close_fds;
static bool flag_devlink_pci;
static bool flag_nic_vf;
static bool flag_vhci_injection;
static bool flag_wifi;
static bool flag_delay_kcov_mmap;

static bool flag_collect_cover;
static bool flag_collect_signal;
static bool flag_dedup_cover;
static bool flag_threaded;
static bool flag_coverage_filter;

static bool flag_turn_on_kssb;
static bool flag_collect_access;

// If true, then executor should write the comparisons data to fuzzer.
static bool flag_comparisons;

// Tunable timeouts, received with execute_req.
static uint64 syscall_timeout_ms;
static uint64 program_timeout_ms;
static uint64 slowdown_scale;

#define SYZ_EXECUTOR 1
#include "common.h"

const int kMaxInput = 4 << 20; // keep in sync with prog.ExecBufferSize
const int kMaxCommands = 1000; // prog package knows about this constant (prog.execMaxCommands)

const uint64 instr_eof = -1;
const uint64 instr_copyin = -2;
const uint64 instr_copyout = -3;
const uint64 instr_setprops = -4;
const uint64 instr_epoch = -5;

const uint64 arg_const = 0;
const uint64 arg_result = 1;
const uint64 arg_data = 2;
const uint64 arg_csum = 3;

const uint64 binary_format_native = 0;
const uint64 binary_format_bigendian = 1;
const uint64 binary_format_strdec = 2;
const uint64 binary_format_strhex = 3;
const uint64 binary_format_stroct = 4;

const uint64 no_copyout = -1;

static int global_epoch;
static int running;
uint32 completed;
bool is_kernel_64_bit = true;

static char* input_data;

// Checksum kinds.
static const uint64 arg_csum_inet = 0;

// Checksum chunk kinds.
static const uint64 arg_csum_chunk_data = 0;
static const uint64 arg_csum_chunk_const = 1;

typedef intptr_t(SYSCALLAPI* syscall_t)(intptr_t, intptr_t, intptr_t, intptr_t, intptr_t, intptr_t, intptr_t, intptr_t, intptr_t);

struct call_t {
	const char* name;
	int sys_nr;
	call_attrs_t attrs;
	syscall_t call;
};

struct cover_t {
	int fd;
	uint32 size;
	enum cov_type type;
	uint32 mmap_alloc_size;
	char* data;
	char* data_end;
	// Note: On everything but darwin the first value in data is the count of
	// recorded PCs, followed by the PCs. We therefore set data_offset to the
	// size of one PC.
	// On darwin data points to an instance of the ksancov_trace struct. Here we
	// set data_offset to the offset between data and the structs 'pcs' member,
	// which contains the PCs.
	intptr_t data_offset;
	// Note: On everything but darwin this is 0, as the PCs contained in data
	// are already correct. XNUs KSANCOV API, however, chose to always squeeze
	// PCs into 32 bit. To make the recorded PC fit, KSANCOV substracts a fixed
	// offset (VM_MIN_KERNEL_ADDRESS for AMD64) and then truncates the result to
	// uint32_t. We get this from the 'offset' member in ksancov_trace.
	intptr_t pc_offset;
};

struct schedule_t {
	uint64_t thread;
	uint64_t addr;
	uint64_t order;
	uint64_t filter;
};

struct thread_t {
	int id;
	bool created;
	event_t ready;
	event_t done;
	event_t start;
	uint64* copyout_pos;
	uint64 copyout_index;
	bool executing;
	int call_index;
	int call_num;
	int num_args;
	intptr_t args[kMaxArgs];
	int epoch;
	uint64 num_sched;
	schedule_t sched[kMaxSchedule];
	uint64 num_filter;
	uint64 footprint[kMaxSchedule];
	bool retry;
	call_props_t call_props;
	intptr_t res;
	uint32 reserrno;
	bool fault_injected;
	cover_t cov;
	cover_t rfcov;
	bool soft_fail_state;
	int cpu;
	bool pending;
};

struct thread_set_t {
	// threads running a call
	struct thread_t set[kMaxFallbackThreads];
	// threads pending a call
	struct thread_t pended[kMaxPendingThreads];
	int blocked;
	int pending;
};

static thread_set_t threads[kMaxThreads];
static thread_t* last_scheduled;
// Threads use this variable to access information about themselves.
static __thread struct thread_t* current_thread;

// #define __EXTRA_RFCOV // TODO: Do we need to make use of extra rfcov?
static cover_t extra_cov;
#ifdef __EXTRA_RFCOV
static cover_t extra_rfcov;
#endif

struct res_t {
	bool executed;
	uint64 val;
};

static res_t results[kMaxCommands];

const uint64 kInMagic = 0xbadc0ffeebadface;
const uint32 kOutMagic = 0xbadf00d;

struct handshake_req {
	uint64 magic;
	uint64 flags; // env flags
	uint64 pid;
	uint64 sandbox_arg;
};

struct handshake_reply {
	uint32 magic;
};

struct execute_req {
	uint64 magic;
	uint64 env_flags;
	uint64 exec_flags;
	uint64 pid;
	uint64 syscall_timeout_ms;
	uint64 program_timeout_ms;
	uint64 slowdown_scale;
	uint64 prog_size;
};

struct execute_reply {
	uint32 magic;
	uint32 done;
	uint32 status;
};

// call_reply.flags
const uint32 call_flag_executed = 1 << 0;
const uint32 call_flag_finished = 1 << 1;
const uint32 call_flag_blocked = 1 << 2;
const uint32 call_flag_fault_injected = 1 << 3;
const uint32 call_flag_retry = 1 << 4;

struct call_reply {
	execute_reply header;
	uint32 magic;
	uint32 call_index;
	uint32 call_num;
	uint32 reserrno;
	uint32 flags;
	uint32 signal_size;
	uint32 cover_size;
	uint32 comps_size;
	// signal/cover/comps follow
};

enum {
	KCOV_CMP_CONST = 1,
	KCOV_CMP_SIZE1 = 0,
	KCOV_CMP_SIZE2 = 2,
	KCOV_CMP_SIZE4 = 4,
	KCOV_CMP_SIZE8 = 6,
	KCOV_CMP_SIZE_MASK = 6,
};

struct kcov_comparison_t {
	// Note: comparisons are always 64-bits regardless of kernel bitness.
	uint64 type;
	uint64 arg1;
	uint64 arg2;
	uint64 pc;

	bool ignore() const;
	void write();
	bool operator==(const struct kcov_comparison_t& other) const;
	bool operator<(const struct kcov_comparison_t& other) const;
};

typedef char kcov_comparison_size[sizeof(kcov_comparison_t) == 4 * sizeof(uint64) ? 1 : -1];

struct feature_t {
	const char* name;
	void (*setup)();
};

struct kssb_flush_table_entry {
	unsigned long inst;
	int value;
	void *pad1, *pad2;
};

static thread_t* get_thread(int thread);
static void prepare_thread(thread_t* th, int call_index, int call_num, uint64 copyout_index, uint64 num_args, uint64* args, uint64 epoch, uint64 num_sched, schedule_t* sched, uint64* pos, call_props_t call_props);
static thread_t* schedule_call(int call_index, int call_num, uint64 copyout_index, uint64 num_args, uint64* args, uint64 thread, uint64 epoch, uint64 num_sched, schedule_t* sched, uint64* pos, call_props_t call_props);
static thread_t* pending_call(int call_index, int call_num, uint64 copyout_index, uint64 num_args, uint64* args, uint64 thread, uint64 epoch, uint64 num_sched, schedule_t* sched, uint64* pos, call_props_t call_props);
static void handle_completion(thread_t* th);
static thread_t* unhand_worker_thread(thread_t* th);
static void copyout_call_results(thread_t* th);
static void write_call_output(thread_t* th, bool finished);
static void write_extra_output();
static void execute_call(thread_t* th);
static void setup_schedule(int num_sched, schedule_t* sched);
static bool clear_schedule(int num_sched, uint64* num_filter, uint64* footprint);
static int lookup_available_cpu(int id);
static void coverage_pre_call(thread_t* th);
static void coverage_post_call(thread_t* th);
static void thread_create(thread_t* th, int id, bool need_coverage);
static void thread_mmap_cover(cover_t* cov);
static void* worker_thread(void* arg);
static uint64 read_input(uint64** input_posp, bool peek = false);
static uint64 read_arg(uint64** input_posp);
static uint64 read_const_arg(uint64** input_posp, uint64* size_p, uint64* bf, uint64* bf_off_p, uint64* bf_len_p);
static uint64 read_result(uint64** input_posp);
static uint64 swap(uint64 v, uint64 size, uint64 bf);
static void copyin(char* addr, uint64 val, uint64 size, uint64 bf, uint64 bf_off, uint64 bf_len);
static bool copyout(char* addr, uint64 size, uint64* res);
static void setup_control_pipes();
static void setup_features(char** enable, int n);
static void setup_affinity_mask(int mask);
static bool __run_in_epoch(uint32 epoch, uint32 global);
static bool run_in_epoch(thread_t* th);
static void feed_flush_vector(int* vector, int vector_size, struct kssb_flush_table_entry* table, int table_size);

#define turn_onoff_kssb_switch()                            \
	do {                                                \
		if (flag_turn_on_kssb) {                    \
			if (syscall(kSSBSwitch))            \
				fail("kssb switch failed"); \
		}                                           \
	} while (0)

#define turn_on_kssb_switch turn_onoff_kssb_switch
#define turn_off_kssb_switch turn_onoff_kssb_switch

#include "syscalls.h"

#if GOOS_linux
#include "executor_linux.h"
#elif GOOS_fuchsia
#include "executor_fuchsia.h"
#elif GOOS_akaros
#include "executor_akaros.h"
#elif GOOS_freebsd || GOOS_netbsd || GOOS_openbsd
#include "executor_bsd.h"
#elif GOOS_darwin
#include "executor_darwin.h"
#elif GOOS_windows
#include "executor_windows.h"
#elif GOOS_test
#include "executor_test.h"
#else
#error "unknown OS"
#endif

#include "cov_filter.h"

#include "test.h"

#if SYZ_HAVE_SANDBOX_ANDROID
static uint64 sandbox_arg = 0;
#endif

int main(int argc, char** argv)
{
	if (argc == 2 && strcmp(argv[1], "version") == 0) {
		puts(GOOS " " GOARCH " " SYZ_REVISION " " GIT_REVISION);
		return 0;
	}
	if (argc >= 2 && strcmp(argv[1], "setup") == 0) {
		setup_features(argv + 2, argc - 2);
		return 0;
	}
	if (argc >= 2 && strcmp(argv[1], "leak") == 0) {
#if SYZ_HAVE_LEAK_CHECK
		check_leaks(argv + 2, argc - 2);
#else
		fail("leak checking is not implemented");
#endif
		return 0;
	}
	if (argc >= 2 && strcmp(argv[1], "setup_kcsan_filterlist") == 0) {
#if SYZ_HAVE_KCSAN
		setup_kcsan_filterlist(argv + 2, argc - 2, true);
#else
		fail("KCSAN is not implemented");
#endif
		return 0;
	}
	if (argc == 2 && strcmp(argv[1], "test") == 0)
		return run_tests();

	if (argc < 2 || strcmp(argv[1], "exec") != 0) {
		fprintf(stderr, "unknown command");
		return 1;
	}

	start_time_ms = current_time_ms();

	os_init(argc, argv, (char*)SYZ_DATA_OFFSET, SYZ_NUM_PAGES * SYZ_PAGE_SIZE);
	current_thread = get_thread(0);
	setup_affinity_mask(kCPUMask[0]);

#if SYZ_EXECUTOR_USES_SHMEM
	void* mmap_out = mmap(NULL, kMaxInput, PROT_READ, MAP_PRIVATE, kInFd, 0);
#else
	void* mmap_out = mmap(NULL, kMaxInput, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANON, -1, 0);
#endif
	if (mmap_out == MAP_FAILED)
		fail("mmap of input file failed");
	input_data = static_cast<char*>(mmap_out);

#if SYZ_EXECUTOR_USES_SHMEM
	mmap_output(kInitialOutput);
	// Prevent test programs to mess with these fds.
	// Due to races in collider mode, a program can e.g. ftruncate one of these fds,
	// which will cause fuzzer to crash.
	close(kInFd);
#if !SYZ_EXECUTOR_USES_FORK_SERVER
	close(kOutFd);
#endif
	// For SYZ_EXECUTOR_USES_FORK_SERVER, close(kOutFd) is invoked in the forked child,
	// after the program has been received.
#endif // if  SYZ_EXECUTOR_USES_SHMEM

	use_temporary_dir();
	install_segv_handler();
	setup_control_pipes();
#if SYZ_EXECUTOR_USES_FORK_SERVER
	receive_handshake();
#else
	receive_execute();
#endif
	if (flag_coverage) {
		for (int i = 0; i < kMaxThreads; i++) {
			thread_set_t* set = &threads[i];
			// No need to init covers for pending
			// threads. We don't run a call in pending
			// threads.
			for (int j = 0; j < kMaxFallbackThreads; j++) {
				thread_t* th = &set->set[j];
				int fd = kCoverFd + ((i * kMaxFallbackThreads) + j) * nr_cov_type;
				cover_init(&th->cov, fd, code_coverage);
				cover_init(&th->rfcov, fd + 1, read_from_coverage);
			}
		}
		extra_cov.fd = kExtraCoverFd;
		cover_open(&extra_cov, true);
		cover_mmap(&extra_cov);
		cover_protect(&extra_cov);
#ifdef __EXTRA_RFCOV
		extra_rfcov.type = read_from_coverage;
		cover_open(&extra_rfcov, true);
		cover_protect(&extra_rfcov);
#endif
		if (flag_extra_coverage) {
			// Don't enable comps because we don't use them in the fuzzer yet.
			cover_enable(&extra_cov, false, true);
		}
#ifdef __EXTRA_RFCOV
		cover_enable(&extra_rfcov, false, false);
#endif
		char sep = '/';
#if GOOS_windows
		sep = '\\';
#endif
		char filename[1024] = {0};
		char* end = strrchr(argv[0], sep);
		size_t len = end - argv[0];
		strncpy(filename, argv[0], len + 1);
		strncat(filename, "syz-cover-bitmap", 17);
		filename[sizeof(filename) - 1] = '\0';
		init_coverage_filter(filename);
	}

	int status = 0;
	if (flag_sandbox_none)
		status = do_sandbox_none();
#if SYZ_HAVE_SANDBOX_SETUID
	else if (flag_sandbox_setuid)
		status = do_sandbox_setuid();
#endif
#if SYZ_HAVE_SANDBOX_NAMESPACE
	else if (flag_sandbox_namespace)
		status = do_sandbox_namespace();
#endif
#if SYZ_HAVE_SANDBOX_ANDROID
	else if (flag_sandbox_android)
		status = do_sandbox_android(sandbox_arg);
#endif
	else
		fail("unknown sandbox type");

#if SYZ_EXECUTOR_USES_FORK_SERVER
	fprintf(stderr, "loop exited with status %d\n", status);
	// Other statuses happen when fuzzer processes manages to kill loop, e.g. with:
	// ptrace(PTRACE_SEIZE, 1, 0, 0x100040)
	if (status != kFailStatus)
		status = 0;
	// If an external sandbox process wraps executor, the out pipe will be closed
	// before the sandbox process exits this will make ipc package kill the sandbox.
	// As the result sandbox process will exit with exit status 9 instead of the executor
	// exit status (notably kFailStatus). So we duplicate the exit status on the pipe.
	reply_execute(status);
	doexit(status);
	// Unreachable.
	return 1;
#else
	reply_execute(status);
	return status;
#endif
}

#if SYZ_EXECUTOR_USES_SHMEM
// This method can be invoked as many times as one likes - MMAP_FIXED can overwrite the previous
// mapping without any problems. The only precondition - kOutFd must not be closed.
static void mmap_output(int size)
{
	if (size <= output_size)
		return;
	if (size % SYZ_PAGE_SIZE != 0)
		failmsg("trying to mmap output area that is not divisible by page size", "page=%d,area=%d", SYZ_PAGE_SIZE, size);
	uint32* mmap_at = NULL;
	if (output_data == NULL) {
		// It's the first time we map output region - generate its location.
		output_data = mmap_at = (uint32*)(kOutputBase + (1 << 20) * (getpid() % 128));
	} else {
		// We are expanding the mmapped region. Adjust the parameters to avoid mmapping already
		// mmapped area as much as possible.
		// There exists a mremap call that could have helped, but it's purely Linux-specific.
		mmap_at = (uint32*)((char*)(output_data) + output_size);
	}
	void* result = mmap(mmap_at, size - output_size,
			    PROT_READ | PROT_WRITE, MAP_SHARED | MAP_FIXED, kOutFd, output_size);
	if (result != mmap_at)
		failmsg("mmap of output file failed", "want %p, got %p", mmap_at, result);
	output_size = size;
}
#endif

void setup_control_pipes()
{
	if (dup2(0, kInPipeFd) < 0)
		fail("dup2(0, kInPipeFd) failed");
	if (dup2(1, kOutPipeFd) < 0)
		fail("dup2(1, kOutPipeFd) failed");
	if (dup2(2, 1) < 0)
		fail("dup2(2, 1) failed");
	// We used to close(0), but now we dup stderr to stdin to keep fd numbers
	// stable across executor and C programs generated by pkg/csource.
	if (dup2(2, 0) < 0)
		fail("dup2(2, 0) failed");
}

void parse_env_flags(uint64 flags)
{
	// Note: Values correspond to ordering in pkg/ipc/ipc.go, e.g. FlagSandboxNamespace
	flag_debug = flags & (1 << 0);
	flag_coverage = flags & (1 << 1);
	if (flags & (1 << 2))
		flag_sandbox_setuid = true;
	else if (flags & (1 << 3))
		flag_sandbox_namespace = true;
	else if (flags & (1 << 4))
		flag_sandbox_android = true;
	else
		flag_sandbox_none = true;
	flag_extra_coverage = false;
	flag_net_injection = flags & (1 << 6);
	flag_net_devices = flags & (1 << 7);
	flag_net_reset = flags & (1 << 8);
	flag_cgroups = flags & (1 << 9);
	flag_close_fds = flags & (1 << 10);
	flag_devlink_pci = flags & (1 << 11);
	flag_vhci_injection = flags & (1 << 12);
	flag_wifi = flags & (1 << 13);
	flag_delay_kcov_mmap = flags & (1 << 14);
	flag_nic_vf = flags & (1 << 15);
}

#if SYZ_EXECUTOR_USES_FORK_SERVER
void receive_handshake()
{
	handshake_req req = {};
	int n = read(kInPipeFd, &req, sizeof(req));
	if (n != sizeof(req))
		failmsg("handshake read failed", "read=%d", n);
	if (req.magic != kInMagic)
		failmsg("bad handshake magic", "magic=0x%llx", req.magic);
#if SYZ_HAVE_SANDBOX_ANDROID
	sandbox_arg = req.sandbox_arg;
#endif
	parse_env_flags(req.flags);
	procid = req.pid;
}

void reply_handshake()
{
	handshake_reply reply = {};
	reply.magic = kOutMagic;
	if (write(kOutPipeFd, &reply, sizeof(reply)) != sizeof(reply))
		fail("control pipe write failed");
}
#endif

static execute_req last_execute_req;

void receive_execute()
{
	execute_req& req = last_execute_req;
	if (read(kInPipeFd, &req, sizeof(req)) != (ssize_t)sizeof(req))
		fail("control pipe read failed");
	if (req.magic != kInMagic)
		failmsg("bad execute request magic", "magic=0x%llx", req.magic);
	if (req.prog_size > kMaxInput)
		failmsg("bad execute prog size", "size=0x%llx", req.prog_size);
	parse_env_flags(req.env_flags);
	procid = req.pid;
	syscall_timeout_ms = req.syscall_timeout_ms;
	program_timeout_ms = req.program_timeout_ms;
	slowdown_scale = req.slowdown_scale;
	flag_collect_signal = req.exec_flags & (1 << 0);
	flag_collect_cover = req.exec_flags & (1 << 1);
	flag_dedup_cover = req.exec_flags & (1 << 2);
	// flag_comparisions might be useful but we don't use it at
	// this point.
	flag_comparisons = false;
	// NOTE: We always enable flag_threaded
	flag_threaded = true;
	flag_coverage_filter = req.exec_flags & (1 << 5);
	flag_turn_on_kssb = req.exec_flags & (1 << 10);
	flag_collect_access = req.exec_flags & (1 << 11);
	if (flag_turn_on_kssb)
		// Enabling kssb makes syscalls about 10x slower.
		slowdown_scale *= 10;

	debug("[%llums] exec opts: procid=%llu threaded=%d cover=%d comps=%d dedup=%d signal=%d"
	      " timeouts=%llu/%llu/%llu prog=%llu filter=%d kssb=%d\n",
	      current_time_ms() - start_time_ms, procid, flag_threaded, flag_collect_cover,
	      flag_comparisons, flag_dedup_cover, flag_collect_signal, syscall_timeout_ms,
	      program_timeout_ms, slowdown_scale, req.prog_size, flag_coverage_filter, flag_turn_on_kssb);
	if (syscall_timeout_ms == 0 || program_timeout_ms <= syscall_timeout_ms || slowdown_scale == 0)
		failmsg("bad timeouts", "syscall=%llu, program=%llu, scale=%llu",
			syscall_timeout_ms, program_timeout_ms, slowdown_scale);
	if (SYZ_EXECUTOR_USES_SHMEM) {
		if (req.prog_size)
			fail("need_prog: no program");
		return;
	}
	if (req.prog_size == 0)
		fail("need_prog: no program");
	uint64 pos = 0;
	for (;;) {
		ssize_t rv = read(kInPipeFd, input_data + pos, kMaxInput - pos);
		if (rv < 0)
			fail("read failed");
		pos += rv;
		if (rv == 0 || pos >= req.prog_size)
			break;
	}
	if (pos != req.prog_size)
		failmsg("bad input size", "size=%lld, want=%lld", pos, req.prog_size);
}

bool cover_collection_required()
{
	return flag_coverage && (flag_collect_signal || flag_collect_cover || flag_comparisons);
}

#if GOOS_akaros
void resend_execute(int fd)
{
	execute_req& req = last_execute_req;
	if (write(fd, &req, sizeof(req)) != sizeof(req))
		fail("child pipe header write failed");
	if (write(fd, input_data, req.prog_size) != (ssize_t)req.prog_size)
		fail("child pipe program write failed");
}
#endif

void reply_execute(int status)
{
	execute_reply reply = {};
	reply.magic = kOutMagic;
	reply.done = true;
	reply.status = status;
	if (write(kOutPipeFd, &reply, sizeof(reply)) != sizeof(reply))
		fail("control pipe write failed");
}

#if SYZ_EXECUTOR_USES_SHMEM
void realloc_output_data()
{
#if SYZ_EXECUTOR_USES_FORK_SERVER
	if (flag_comparisons)
		mmap_output(kMaxOutputComparisons);
	else if (flag_collect_cover)
		mmap_output(kMaxOutputCoverage);
	else if (flag_collect_signal)
		mmap_output(kMaxOutputSignal);
	if (close(kOutFd) < 0)
		fail("failed to close kOutFd");
#endif
}
#endif // if SYZ_EXECUTOR_USES_SHMEM

static void prepare_schedule(void)
{
	bool need_prepare = false;
	int num_schedpoints = 0;
	int num_cpus = 0;
	for (int i = 0; i < kMaxThreads; i++) {
		thread_t* th = get_thread(i);
		if (!th->created || !event_isset(&th->ready) || th->num_sched == 0 || !run_in_epoch(th))
			continue;
		num_cpus++;
		num_schedpoints += th->num_sched;
		need_prepare = true;
	}
	if (!need_prepare)
		return;
	WARN_ON_NOT_NULL(hypercall(HCALL_PREPARE, num_schedpoints, num_cpus, 0), "HCALL_PREPARE");
}

void resume_pending_call(int thread, thread_t* pended)
{
	debug("resume a call %d@%d\n", thread, pended->epoch);
	pended->pending = false;
	schedule_call(pended->call_index, pended->call_num, pended->copyout_index,
		      pended->num_args, (uint64*)pended->args, thread, pended->epoch, pended->num_sched, pended->sched, pended->copyout_pos, pended->call_props);
}

void resume_pending_calls()
{
	for (int i = 0; i < kMaxThreads; i++) {
		thread_set_t* set = &threads[i];
		if (set->pending == 0)
			continue;
		int idx;
		for (idx = 0; idx < kMaxPendingThreads; idx++)
			if (set->pended[idx].pending)
				break;
		if (idx == kMaxPendingThreads)
			// This should be an error
			continue;
		thread_t* th = &set->pended[idx];
		if (!run_in_epoch(th))
			continue;
		resume_pending_call(i, th);
	}
}

// start_epoch() assumes schedule_call() gracefully handles a blocking
// thread, and the forefront thread can execute an assigned call.
void start_epoch()
{
	uint64_t timeout = 0;
	// Signal threads that are ready to execute. Each thread will
	// reset th->ready after th->start is set.
	debug("start epoch %d\n", global_epoch);
	prepare_schedule();
	for (int i = 0; i < kMaxThreads; i++) {
		thread_t* th = get_thread(i);
		if (th->created && event_isset(&th->ready)) {
			if (event_isset(&th->start) || event_isset(&th->done) || !th->executing)
				failmsg("bad thread state in start_epoch", "start=%d done=%d executing=%d",
					event_isset(&th->start), event_isset(&th->done), th->executing);
			const call_t* call = &syscalls[th->call_num];
			if (timeout < call->attrs.timeout)
				timeout = call->attrs.timeout;
			event_set(&th->start);
		}
	}

	uint64 timeout_ms = syscall_timeout_ms + timeout * slowdown_scale;
	if (flag_debug && timeout_ms < 1000)
		timeout_ms = 1000;

	// handle completion for all threads if any
	for (int i = 0; i < kMaxThreads; i++) {
		thread_t* th = get_thread(i);
		if (!th->created)
			continue;
		if (th->executing) {
			// Let's wait more if calls are racing.
			int scheduling_scale = (th->num_sched != 0 ? 3 : 1);
			if (event_timedwait(&th->done, timeout_ms * scheduling_scale) && run_in_epoch(th))
				handle_completion(th);
			else
				// since we waited for this syscall
				// for a some amount of time, we don't
				// need to wait later syscalls long.
				timeout_ms = 1;
		}
	}
	global_epoch++;
}

void wait_epoch(thread_t* th)
{
	do {
		debug("wait epoch %d\n", th->epoch);
		event_wait(&th->start);
		event_reset(&th->start);
	} while (!run_in_epoch(th));
}

bool __run_in_epoch(uint32 epoch, uint32 global)
{
	return epoch <= global;
}

bool run_in_epoch(thread_t* th)
{
	// return true if th can execute a call in the global epoch
	return __run_in_epoch(th->epoch, global_epoch);
}

void prepare_kssb(void)
{
	int num_th = (flag_turn_on_kssb ? 2 : 1);
	// After enabling kssb, syscalls can be slow down 10x, meaning
	// creating threads also takes so much time. Preparew worker
	// threads to reduce the thread-creating latency.
	for (int i = 0; i < num_th; i++) {
		thread_t* th = get_thread(i);
		if (!th->created)
			thread_create(th, i, cover_collection_required());
	}
	WARN_ON_NOT_NULL(hypercall(HCALL_RESET, 0, 0, 0), "HCALL_RESET");
	if (flag_collect_access)
		enable_kssb();
}

void finalize_kssb(void)
{
	if (flag_collect_access)
		disable_kssb();
}

// execute_one executes program stored in input_data.
void execute_one()
{
	prepare_kssb();
#if SYZ_EXECUTOR_USES_SHMEM
	realloc_output_data();
	output_pos = output_data;
	write_output(0); // Number of executed syscalls (updated later).
#endif // if SYZ_EXECUTOR_USES_SHMEM
	uint64 start = current_time_ms();
	uint64* input_pos = (uint64*)input_data;

	if (cover_collection_required()) {
		if (!flag_threaded)
			// XXX: In this project, flag_threaded is
			// always true, so we can safely remove this if block
			cover_enable(&get_thread(0)->cov, flag_comparisons, false);
		if (flag_extra_coverage)
			cover_reset(&extra_cov);
	}

	int call_index = 0;
	uint64 prog_extra_timeout = 0;
	uint64 prog_extra_cover_timeout = 0;
	call_props_t call_props;
	memset(&call_props, 0, sizeof(call_props));
	int filter_size, vector_size, table_size;
	int filter[kMaxSchedule] = {
	    0,
	};
	int vector[kMaxVector] = {
	    0,
	};
	struct kssb_flush_table_entry table[kMaxVector] = {
	    0,
	};

	filter_size = (int)read_input(&input_pos);
	for (int i = 0; i < filter_size; i++) {
		int f = (int)read_input(&input_pos);
		if (i >= kMaxSchedule)
			continue;
		filter[i] = f;
	}
	if (filter_size > kMaxSchedule)
		filter_size = kMaxSchedule;

	vector_size = (int)read_input(&input_pos);
	for (int i = 0; i < vector_size; i++)
		vector[i] = (int)read_input(&input_pos);
	table_size = (int)read_input(&input_pos) / 2;
	for (int i = 0; i < table_size; i++) {
		unsigned long inst = read_input(&input_pos);
		int value = read_input(&input_pos);
		table[i] = {.inst = inst, .value = value};
	}
	feed_flush_vector(vector, vector_size, table, table_size);

	for (;;) {
		uint64 call_num = read_input(&input_pos);
		if (call_num == instr_eof)
			break;
		if (call_num == instr_epoch) {
			resume_pending_calls();
			start_epoch();
			continue;
		}
		if (call_num == instr_copyin) {
			char* addr = (char*)read_input(&input_pos);
			uint64 typ = read_input(&input_pos);
			switch (typ) {
			case arg_const: {
				uint64 size, bf, bf_off, bf_len;
				uint64 arg = read_const_arg(&input_pos, &size, &bf, &bf_off, &bf_len);
				copyin(addr, arg, size, bf, bf_off, bf_len);
				break;
			}
			case arg_result: {
				uint64 meta = read_input(&input_pos);
				uint64 size = meta & 0xff;
				uint64 bf = meta >> 8;
				uint64 val = read_result(&input_pos);
				copyin(addr, val, size, bf, 0, 0);
				break;
			}
			case arg_data: {
				uint64 size = read_input(&input_pos);
				size &= ~(1ull << 63); // readable flag
				NONFAILING(memcpy(addr, input_pos, size));
				// Read out the data.
				for (uint64 i = 0; i < (size + 7) / 8; i++)
					read_input(&input_pos);
				break;
			}
			case arg_csum: {
				debug_verbose("checksum found at %p\n", addr);
				uint64 size = read_input(&input_pos);
				char* csum_addr = addr;
				uint64 csum_kind = read_input(&input_pos);
				switch (csum_kind) {
				case arg_csum_inet: {
					if (size != 2)
						failmsg("bag inet checksum size", "size=%llu", size);
					debug_verbose("calculating checksum for %p\n", csum_addr);
					struct csum_inet csum;
					csum_inet_init(&csum);
					uint64 chunks_num = read_input(&input_pos);
					uint64 chunk;
					for (chunk = 0; chunk < chunks_num; chunk++) {
						uint64 chunk_kind = read_input(&input_pos);
						uint64 chunk_value = read_input(&input_pos);
						uint64 chunk_size = read_input(&input_pos);
						switch (chunk_kind) {
						case arg_csum_chunk_data:
							debug_verbose("#%lld: data chunk, addr: %llx, size: %llu\n",
								      chunk, chunk_value, chunk_size);
							NONFAILING(csum_inet_update(&csum, (const uint8*)chunk_value, chunk_size));
							break;
						case arg_csum_chunk_const:
							if (chunk_size != 2 && chunk_size != 4 && chunk_size != 8)
								failmsg("bad checksum const chunk size", "size=%lld", chunk_size);
							// Here we assume that const values come to us big endian.
							debug_verbose("#%lld: const chunk, value: %llx, size: %llu\n",
								      chunk, chunk_value, chunk_size);
							csum_inet_update(&csum, (const uint8*)&chunk_value, chunk_size);
							break;
						default:
							failmsg("bad checksum chunk kind", "kind=%llu", chunk_kind);
						}
					}
					uint16 csum_value = csum_inet_digest(&csum);
					debug_verbose("writing inet checksum %hx to %p\n", csum_value, csum_addr);
					copyin(csum_addr, csum_value, 2, binary_format_native, 0, 0);
					break;
				}
				default:
					failmsg("bad checksum kind", "kind=%llu", csum_kind);
				}
				break;
			}
			default:
				failmsg("bad argument type", "type=%llu", typ);
			}
			continue;
		}
		if (call_num == instr_copyout) {
			read_input(&input_pos); // index
			read_input(&input_pos); // addr
			read_input(&input_pos); // size
			// The copyout will happen when/if the call completes.
			continue;
		}
		if (call_num == instr_setprops) {
			read_call_props_t(call_props, read_input(&input_pos, false));
			continue;
		}

		// Normal syscall.
		if (call_num >= ARRAY_SIZE(syscalls))
			failmsg("invalid syscall number", "call_num=%llu", call_num);
		const call_t* call = &syscalls[call_num];
		if (call->attrs.disabled)
			failmsg("executing disabled syscall", "syscall=%s", call->name);
		if (prog_extra_timeout < call->attrs.prog_timeout)
			prog_extra_timeout = call->attrs.prog_timeout * slowdown_scale;
		if (strncmp(syscalls[call_num].name, "syz_usb", strlen("syz_usb")) == 0)
			prog_extra_cover_timeout = std::max(prog_extra_cover_timeout, 500 * slowdown_scale);
		if (strncmp(syscalls[call_num].name, "syz_80211_inject_frame", strlen("syz_80211_inject_frame")) == 0)
			prog_extra_cover_timeout = std::max(prog_extra_cover_timeout, 300 * slowdown_scale);
		uint64 copyout_index = read_input(&input_pos);
		uint64 num_args = read_input(&input_pos);
		if (num_args > kMaxArgs)
			failmsg("command has bad number of arguments", "args=%llu", num_args);
		uint64 args[kMaxArgs] = {};
		for (uint64 i = 0; i < num_args; i++)
			args[i] = read_arg(&input_pos);
		for (uint64 i = num_args; i < kMaxArgs; i++)
			args[i] = 0;
		uint64 thread = read_input(&input_pos);
		uint64 epoch = read_input(&input_pos);
		uint64 num_sched = read_input(&input_pos);
		schedule_t sched[kMaxSchedule];
		for (uint64 i = 0; i < num_sched; i++) {
			uint64 thread, addr, order;
			thread = read_input(&input_pos);
			addr = read_input(&input_pos);
			order = read_input(&input_pos);
			if (i >= kMaxSchedule)
				continue;
			sched[i].thread = thread;
			sched[i].addr = addr;
			sched[i].order = order;
			sched[i].filter = (order < kMaxSchedule ? filter[order] : 1);
		}
		if (num_sched > kMaxSchedule)
			num_sched = kMaxSchedule;
		schedule_call(call_index++, call_num, copyout_index,
			      num_args, args, thread, epoch, num_sched, sched, input_pos, call_props);
		memset(&call_props, 0, sizeof(call_props));
	}

	if (running > 0) {
		// Give unfinished syscalls some additional time.
		last_scheduled = 0;
		uint64 wait_start = current_time_ms();
		uint64 wait_end = wait_start + 2 * syscall_timeout_ms;
		wait_end = std::max(wait_end, start + program_timeout_ms / 6);
		wait_end = std::max(wait_end, wait_start + prog_extra_timeout);
		while (running > 0 && current_time_ms() <= wait_end) {
			sleep_ms(1 * slowdown_scale);
			// Do not handle completion of pending threads. they don't
			// run a call.
			for (int i = 0; i < kMaxThreads; i++) {
				thread_set_t* set = &threads[i];
				for (int j = 0; j < kMaxFallbackThreads; j++) {
					thread_t* th = &set->set[j];
					if (!th->created)
						continue;
					if (th->executing && event_isset(&th->done))
						handle_completion(th);
				}
			}
		}
		// Write output coverage for unfinished calls.
		if (running > 0) {
			for (int i = 0; i < kMaxThreads; i++) {
				thread_set_t* set = &threads[i];
				// Pending threads don't run a call
				for (int j = 0; j < kMaxFallbackThreads; j++) {
					thread_t* th = &set->set[j];
					if (th->executing) {
						if (flag_coverage) {
							cover_collect(&th->cov);
							cover_collect(&th->rfcov);
						}
						write_call_output(th, false);
					}
				}
			}
		}
	}

#if SYZ_HAVE_CLOSE_FDS
	close_fds();
#endif

	write_extra_output();
	// Check for new extra coverage in small intervals to avoid situation
	// that we were killed on timeout before we write any.
	// Check for extra coverage is very cheap, effectively a memory load.
	const uint64 kSleepMs = 100;
	for (uint64 i = 0; i < prog_extra_cover_timeout / kSleepMs; i++) {
		sleep_ms(kSleepMs);
		write_extra_output();
	}
	finalize_kssb();
}

void feed_flush_vector(int* vector, int vector_size, struct kssb_flush_table_entry* table, int table_size)
{
	// XXX: Not sure we don't have to do this
	if (vector_size == 0 && table_size != 0) {
		vector_size = 1;
		vector[0] = 1;
	}
#define SYS_FEEDINPUT 500
#define DEBUG_FLUSH_VECTOR
#ifdef DEBUG_FLUSH_VECTOR
	debug("vector: %d [", vector_size);
	for (int i = 0; i < vector_size; i++) {
		if (i != 0)
			debug_noprefix(", ");
		debug_noprefix("%d", vector[i]);
	}
	debug_noprefix("]\n");
	debug("table: %d [", table_size);
	for (int i = 0; i < table_size; i++) {
		if (i != 0)
			debug_noprefix(", ");
		debug_noprefix("%lx: %d", table[i].inst, table[i].value);
	}
	debug_noprefix("]\n");
#endif
	if (syscall(SYS_FEEDINPUT, vector, vector_size, table, table_size))
		fail("feedinput failed");
}

// Get a thread to run a call
// NOTE: get_thread() should be called in the main thread get_thread()
// does not care that the forefront thread is blocked or not.
thread_t* get_thread(int thread)
{
	thread_set_t* set = &threads[thread];
	int idx = set->blocked;
	if (idx >= kMaxFallbackThreads)
		exitf("out of threads (id #%d)\n", thread);
	return &set->set[idx];
}

void prepare_thread(thread_t* th, int call_index, int call_num, uint64 copyout_index, uint64 num_args, uint64* args, uint64 epoch, uint64 num_sched, schedule_t* sched, uint64* pos, call_props_t call_props)
{
	th->copyout_pos = pos;
	th->copyout_index = copyout_index;
	event_reset(&th->done);
	th->executing = true;
	th->call_index = call_index;
	th->call_num = call_num;
	th->num_args = num_args;
	th->call_props = call_props;
	for (int i = 0; i < kMaxArgs; i++)
		th->args[i] = args[i];
	th->epoch = epoch;
	th->num_sched = num_sched;
	for (uint64 i = 0; i < num_sched; i++)
		th->sched[i] = sched[i];
}

thread_t* pending_call(int call_index, int call_num, uint64 copyout_index, uint64 num_args, uint64* args, uint64 thread, uint64 epoch, uint64 num_sched, schedule_t* sched, uint64* pos, call_props_t call_props)
{
	thread_set_t* set = &threads[thread];
	if (set->pending == kMaxPendingThreads)
		// We reach the maximum number of pending calls. Drop
		// the call.
		return NULL;
	int idx = set->pending++;
	thread_t* th = &set->pended[idx];
	prepare_thread(th, call_index, call_num, copyout_index,
		       num_args, args, epoch, num_sched, sched, pos, call_props);
	th->pending = true;
	return th;
}

thread_t* schedule_call(int call_index, int call_num, uint64 copyout_index, uint64 num_args, uint64* args, uint64 thread, uint64 epoch, uint64 num_sched, schedule_t* sched, uint64* pos, call_props_t call_props)
{
	debug("schedule a call (%d) to thread %llu@%llu\n", call_index, thread, epoch);
	if (!__run_in_epoch(epoch, global_epoch)) {
		// It is too early to schedule this call. Let's
		// pending the call
		debug("pending a call %llu@%llu\n", thread, epoch);
		return pending_call(call_index, call_num, copyout_index, num_args, args, thread, epoch, num_sched, sched, pos, call_props);
	}
	thread_t* th = get_thread(thread);
	if (!th->created)
		thread_create(th, thread, cover_collection_required());
	if (th->executing) {
		if (event_isset(&th->done))
			// The worker thread notify the main thread
			// that it has been finished. The main thread
			// can handle the completion.
			handle_completion(th);
		else
			// The worker thread is either blocked or
			// still executing the call. Drop the worker
			// thread so remaining calls can be executed
			// normally.
			th = unhand_worker_thread(th);
	}
	if (th == NULL)
		// unhand_worker_thread failed to find a fallback
		// thread. drop the call.
		return NULL;
	if (event_isset(&th->ready) || !event_isset(&th->done) || th->executing)
		failmsg("bad thread state in schedule", "ready=%d done=%d executing=%d",
			event_isset(&th->ready), event_isset(&th->done), th->executing);
	last_scheduled = th;
	prepare_thread(th, call_index, call_num, copyout_index,
		       num_args, args, epoch, num_sched, sched, pos, call_props);

	event_set(&th->ready);
	running++;
	return th;
}

#if SYZ_EXECUTOR_USES_SHMEM
template <typename cover_data_t>
void write_code_coverage_signal(cover_t* cov, uint32* signal_count_pos, uint32* cover_count_pos)
{
	// Write out feedback signals.
	// Currently it is code edges computed as xor of two subsequent basic block PCs.
	cover_data_t* cover_data = (cover_data_t*)(cov->data + cov->data_offset);
	if (flag_collect_signal) {
		uint32 nsig = 0;
		cover_data_t prev_pc = 0;
		bool prev_filter = true;
		for (uint32 i = 0; i < cov->size; i++) {
			cover_data_t pc = cover_data[i] + cov->pc_offset;
			uint32 sig = pc;
			if (use_cover_edges(pc))
				sig ^= hash(prev_pc);
			bool filter = coverage_filter(pc);
			// Ignore the edge only if both current and previous PCs are filtered out
			// to capture all incoming and outcoming edges into the interesting code.
			bool ignore = !filter && !prev_filter;
			prev_pc = pc;
			prev_filter = filter;
			if (ignore || dedup(sig))
				continue;
			write_output(sig);
			nsig++;
		}
		// Write out number of signals.
		*signal_count_pos = nsig;
	}

	if (flag_collect_cover) {
		// Write out real coverage (basic block PCs).
		uint32 cover_size = cov->size;
		if (flag_dedup_cover) {
			cover_data_t* end = cover_data + cover_size;
			cover_unprotect(cov);
			std::sort(cover_data, end);
			cover_size = std::unique(cover_data, end) - cover_data;
			cover_protect(cov);
		}
		// Truncate PCs to uint32 assuming that they fit into 32-bits.
		// True for x86_64 and arm64 without KASLR.
		for (uint32 i = 0; i < cover_size; i++)
			write_output(cover_data[i] + cov->pc_offset);
		*cover_count_pos = cover_size;
	}
}

template <typename cover_data_t>
void write_read_from_coverage_signal(cover_t* cov, uint32* signal_count_pos, uint32* cover_count_pos)
{
	if (signal_count_pos)
		// The signal for read-from coverage is not well
		// defined yet. Just ignore it.
		failmsg("bad signal_count_pos in write_read_from_coverage_signal",
			"signal_count_pos=%p", signal_count_pos);

	uint32 cover_size = cov->size;
	struct kmemcov_access* cover_data = &((struct kmemcov_access*)cov->data)[1];
	for (uint32 i = 0; i < cover_size; i++) {
		// Truncate all fields into uint32. This is fine for
		// inst, size, type, and timestamp, but truncating
		// addr may introduce the possibility that for two
		// memory accesses that did not access the same memory
		// object, our fuzzer thinks they did access the same
		// memory object. Well, whatever, this is a fuzzer.
		write_output(cover_data[i].inst);
		write_output(cover_data[i].addr);
		write_output(cover_data[i].size);
		write_output(cover_data[i].type);
		write_output(cover_data[i].timestamp);
	}
	*cover_count_pos = cover_size;
}

template <typename cover_data_t>
void write_coverage_signal(cover_t* cov, uint32* signal_count_pos, uint32* cover_count_pos)
{
	if (cov->type == code_coverage)
		write_code_coverage_signal<cover_data_t>(cov, signal_count_pos, cover_count_pos);
	else
		write_read_from_coverage_signal<cover_data_t>(cov, signal_count_pos, cover_count_pos);
}
#endif // if SYZ_EXECUTOR_USES_SHMEM

// NOTE: handle_completion() should be called in the main thread
void handle_completion(thread_t* th)
{
#if 0
	debug("handle_completion: th #%2d\n", th->id);
#endif
	if (event_isset(&th->ready) || !event_isset(&th->done) || !th->executing)
		failmsg("bad thread state in completion", "ready=%d done=%d executing=%d",
			event_isset(&th->ready), event_isset(&th->done), th->executing);
	if (th->res != (intptr_t)-1)
		copyout_call_results(th);

	write_call_output(th, true);
	write_extra_output();
	th->retry = false;
	th->executing = false;
	running--;
	if (running < 0) {
		// This fires periodically for the past 2 years (see issue #502).
		fprintf(stderr, "running=%d completed=%d flag_threaded=%d current=%d\n",
			running, completed, flag_threaded, th->id);
		for (int i = 0; i < kMaxThreads; i++) {
			thread_set_t* set = &threads[i];
			for (int j = 0; j < kMaxFallbackThreads; j++) {
				thread_t* th1 = &set->set[j];
				fprintf(stderr, "th #%2d: created=%d executing=%d"
						" ready=%d done=%d call_index=%d res=%lld reserrno=%d\n",
					i, th1->created, th1->executing,
					event_isset(&th1->ready), event_isset(&th1->done),
					-th1->call_index, (uint64)th1->res, th1->reserrno);
			}
		}
		exitf("negative running");
	}
}

// NOTE: unhand_worker_thread() should be called in the main thread
thread_t* unhand_worker_thread(thread_t* th)
{
	if (!event_isset(&th->done)) {
		// The main thread needs to schedule a new call to th,
		// while th is still executing a previous call. Assume
		// that th is blocked, and find the next fallback
		// thread to replace the previous one.
		int idx, id = th->id;
		thread_set_t* set = &threads[id];
		if (set->blocked == kMaxFallbackThreads) {
			debug("failed to unhand the worker thread %d (out of threads), drop the call\n", id);
			return NULL;
		}
		idx = ++set->blocked;
		debug("unhand the worker thread %d, blocked=%d\n", id, set->blocked);
		th = &set->set[idx];
		if (th->created)
			failmsg("bad thread state in get_thread", "created=%d", th->created);
		thread_create(th, id, cover_collection_required());
	}
	return th;
}

void copyout_call_results(thread_t* th)
{
	if (th->copyout_index != no_copyout) {
		if (th->copyout_index >= kMaxCommands)
			failmsg("result overflows kMaxCommands", "index=%lld", th->copyout_index);
		results[th->copyout_index].executed = true;
		results[th->copyout_index].val = th->res;
	}
	for (bool done = false; !done;) {
		uint64 instr = read_input(&th->copyout_pos);
		switch (instr) {
		case instr_copyout: {
			uint64 index = read_input(&th->copyout_pos);
			if (index >= kMaxCommands)
				failmsg("result overflows kMaxCommands", "index=%lld", index);
			char* addr = (char*)read_input(&th->copyout_pos);
			uint64 size = read_input(&th->copyout_pos);
			uint64 val = 0;
			if (copyout(addr, size, &val)) {
				results[index].executed = true;
				results[index].val = val;
			}
			debug_verbose("copyout 0x%llx from %p\n", val, addr);
			break;
		}
		default:
			done = true;
			break;
		}
	}
}

void write_call_output(thread_t* th, bool finished)
{
	uint32 reserrno = 999;
	const bool blocked = finished && th != last_scheduled;
	uint32 call_flags = call_flag_executed | (blocked ? call_flag_blocked : 0);
	uint32 num_filter = (uint32)th->num_filter;
	if (finished) {
		reserrno = th->res != -1 ? 0 : th->reserrno;
		call_flags |= call_flag_finished |
			      (th->fault_injected ? call_flag_fault_injected : 0);
	}
	call_flags |= (th->retry ? call_flag_retry : 0);
#if SYZ_EXECUTOR_USES_SHMEM
	write_output(kOutMagic);
	write_output(th->call_index);
	write_output(th->call_num);
	write_output(reserrno);
	write_output(call_flags);
	uint32* signal_count_pos = write_output(0); // filled in later
	uint32* cover_count_pos = write_output(0); // filled in later
	uint32* comps_count_pos = write_output(0); // filled in later
	uint32* rfcover_count_pos = write_output(0); // filled in later
	write_output(num_filter);

	if (flag_comparisons) {
		// Collect only the comparisons
		uint32 ncomps = th->cov.size;
		kcov_comparison_t* start = (kcov_comparison_t*)(th->cov.data + sizeof(uint64));
		kcov_comparison_t* end = start + ncomps;
		if ((char*)end > th->cov.data_end)
			failmsg("too many comparisons", "ncomps=%u", ncomps);
		cover_unprotect(&th->cov);
		std::sort(start, end);
		ncomps = std::unique(start, end) - start;
		cover_protect(&th->cov);
		uint32 comps_size = 0;
		for (uint32 i = 0; i < ncomps; ++i) {
			if (start[i].ignore())
				continue;
			comps_size++;
			start[i].write();
		}
		// Write out number of comparisons.
		*comps_count_pos = comps_size;
	} else if (flag_collect_signal || flag_collect_cover) {
		if (is_kernel_64_bit) {
			write_coverage_signal<uint64>(&th->cov, signal_count_pos, cover_count_pos);
			write_coverage_signal<uint64>(&th->rfcov, NULL, rfcover_count_pos);
		} else {
			// XXX: We support only 64 bit kernel.
			write_coverage_signal<uint32>(&th->cov, signal_count_pos, cover_count_pos);
		}
	}
	for (int i = 0; i < (int)num_filter; i++) {
		write_output((uint32)th->sched[i].order);
		write_output((uint32)th->footprint[i]);
	}
	debug_verbose("out #%u: index=%u num=%u errno=%d finished=%d blocked=%d sig=%u cover=%u comps=%u\n",
		      completed, th->call_index, th->call_num, reserrno, finished, blocked,
		      *signal_count_pos, *cover_count_pos, *comps_count_pos);
	completed++;
	write_completed(completed);
#else
	call_reply reply;
	reply.header.magic = kOutMagic;
	reply.header.done = 0;
	reply.header.status = 0;
	reply.magic = kOutMagic;
	reply.call_index = th->call_index;
	reply.call_num = th->call_num;
	reply.reserrno = reserrno;
	reply.flags = call_flags;
	reply.signal_size = 0;
	reply.cover_size = 0;
	reply.comps_size = 0;
	if (write(kOutPipeFd, &reply, sizeof(reply)) != sizeof(reply))
		fail("control pipe call write failed");
	debug_verbose("out: index=%u num=%u errno=%d finished=%d blocked=%d\n",
		      th->call_index, th->call_num, reserrno, finished, blocked);
#endif // if SYZ_EXECUTOR_USES_SHMEM
}

void write_extra_output()
{
#if SYZ_EXECUTOR_USES_SHMEM
	if (!cover_collection_required() || !flag_extra_coverage || flag_comparisons)
		return;
	cover_collect(&extra_cov);
	if (!extra_cov.size)
		return;
	write_output(kOutMagic);
	write_output(-1); // call index
	write_output(-1); // call num
	write_output(999); // errno
	write_output(0); // call flags
	uint32* signal_count_pos = write_output(0); // filled in later
	uint32* cover_count_pos = write_output(0); // filled in later
	write_output(0); // comps_count_pos
	if (is_kernel_64_bit)
		write_coverage_signal<uint64>(&extra_cov, signal_count_pos, cover_count_pos);
	else
		write_coverage_signal<uint32>(&extra_cov, signal_count_pos, cover_count_pos);
	cover_reset(&extra_cov);
	debug_verbose("extra: sig=%u cover=%u\n", *signal_count_pos, *cover_count_pos);
	completed++;
	write_completed(completed);
#endif // if SYZ_EXECUTOR_USES_SHMEM
}

void thread_create(thread_t* th, int id, bool need_coverage)
{
	debug("creating a thread %d (need_coverage: %d)\n", id, need_coverage);
	th->created = true;
	th->id = id;
	th->executing = false;
	// Lazily set up coverage collection.
	// It is assumed that actually it's already initialized - with a few rare exceptions.
	if (need_coverage) {
		if (!th->cov.fd)
			exitf("out of opened kcov threads");
		thread_mmap_cover(&th->cov);
		thread_mmap_cover(&th->rfcov);
	}
	event_init(&th->ready);
	event_init(&th->done);
	event_init(&th->start);
	event_set(&th->done);
	if (flag_threaded)
		thread_start(worker_thread, th);
}

void thread_mmap_cover(cover_t* cov)
{
	if (cov->data != NULL)
		return;
	cover_mmap(cov);
	cover_protect(cov);
}

void* worker_thread(void* arg)
{
	thread_t* th = (thread_t*)arg;
	threadid = th->id;
	current_thread = th;
	th->cpu = lookup_available_cpu(th->id);
	setup_affinity_mask(kCPUMask[th->cpu]);
	if (cover_collection_required()) {
		cover_enable(&th->cov, flag_comparisons, false);
		cover_enable(&th->rfcov, false, false);
	}
	for (;;) {
		event_wait(&th->ready);
		// The main thread will notify th to start the
		// execution. Worker threads can reset th->ready only after
		// the notification.
		wait_epoch(th);
		event_reset(&th->ready);
		execute_call(th);
		event_set(&th->done);
	}
	return 0;
}

void execute_call(thread_t* th)
{
	const call_t* call = &syscalls[th->call_num];
	debug("call #%d@%d [%llums] -> %s(",
	      th->id, th->epoch, current_time_ms() - start_time_ms, call->name);
	for (int i = 0; i < th->num_args; i++) {
		if (i != 0)
			debug_noprefix(", ");
		debug_noprefix("0x%llx", (uint64)th->args[i]);
	}
	debug_noprefix(")\n");

	int fail_fd = -1;
	th->soft_fail_state = false;
	if (th->call_props.fail_nth > 0) {
		if (th->call_props.rerun > 0)
			fail("both fault injection and rerun are enabled for the same call");
		fail_fd = inject_fault(th->call_props.fail_nth);
		th->soft_fail_state = true;
	}

	// For pseudo-syscalls and user-space functions NONFAILING can abort before assigning to th->res.
	// Arrange for res = -1 and errno = EFAULT result for such case.
	th->res = -1;
	errno = EFAULT;
	// Turn on the ssb switch
	turn_on_kssb_switch();
	setup_schedule(th->num_sched, th->sched);
	coverage_pre_call(th);
	NONFAILING(th->res = execute_syscall(call, th->args));
	coverage_post_call(th);
	th->retry = clear_schedule(th->num_sched, &th->num_filter, th->footprint);
	// Then turn off the ssb switch
	turn_off_kssb_switch();
	th->reserrno = errno;
	// Our pseudo-syscalls may misbehave.
	if ((th->res == -1 && th->reserrno == 0) || call->attrs.ignore_return)
		th->reserrno = EINVAL;
	// Reset the flag before the first possible fail().
	th->soft_fail_state = false;

	th->fault_injected = false;

	if (th->call_props.fail_nth > 0)
		th->fault_injected = fault_injected(fail_fd);

	// If required, run the syscall some more times.
	// But let's still return res, errno and coverage from the first execution.
	for (int i = 0; i < th->call_props.rerun; i++)
		NONFAILING(execute_syscall(call, th->args));

	debug("call #%d@%d [%llums] <- %s=0x%llx ",
	      th->id, th->epoch, current_time_ms() - start_time_ms, call->name, (uint64)th->res);
	if (th->res == (intptr_t)-1)
		debug_noprefix(" errno=%d", th->reserrno);
	if (flag_coverage)
		debug_noprefix(" cover=%u rfcov=%u", th->cov.size, th->rfcov.size);
	if (th->call_props.fail_nth > 0)
		debug_noprefix(" fault=%d", th->fault_injected);
	if (th->call_props.rerun > 0)
		debug_noprefix(" rerun=%d", th->call_props.rerun);
	debug_noprefix("\n");
	setup_affinity_mask(kCPUMask[th->cpu]);
}

void setup_schedule(int num_sched, schedule_t* sched)
{
	if (num_sched == 0)
		return;
	debug("installing breakpoint bp=%d\n", num_sched);
	for (int i = 0; i < num_sched; i++) {
		WARN_ON_NOT_NULL(hypercall(HCALL_INSTALL_BP, sched[i].addr,
					   sched[i].order, sched[i].filter),
				 "HCALL_INSTALL_BP");
	}

	int attempt = 10;
	uint64 res = hypercall(HCALL_ACTIVATE_BP, 0, 0, 0);
	while (res == (uint64)(-EAGAIN) && --attempt) {
		sleep_ms(10);
		res = hypercall(HCALL_ACTIVATE_BP, 0, 0, 0);
	}
	if (res != 0)
		debug("failed to setup a schedule: %llx\n", res);
}

bool clear_schedule(int num_sched, uint64* num_filter, uint64* footprint)
{
	if (num_sched == 0)
		return false;

	uint64_t count;
	uint64_t retry;
	unsigned long ret;

	WARN_ON_NOT_NULL(hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0), "HCALL_DEACTIVATE_BP");
	WARN_ON_NOT_NULL((ret = hypercall(HCALL_FOOTPRINT_BP, (unsigned long)&count, (unsigned long)footprint,
					  (unsigned long)&retry)),
			 "HCALL_FOOTPRINT_BP");
	if (ret != 0)
		count = 0;
	if ((int)count > num_sched)
		debug("[WARN] count > num_sched, count=%d, num_sched=%d\n", (int)count, num_sched);
	*num_filter = count;
	WARN_ON_NOT_NULL(hypercall(HCALL_CLEAR_BP, 0, 0, 0), "HCALL_CLEAR_BP");
	return !!retry;
}

int lookup_available_cpu(int id)
{
	// TODO: Currently, we are testing to trigger CVE-2017-2636
	// with only one worker process, so it is okay to statically
	// assign a thread to a cpu. This limits the number of
	// processes, so fix this.
	return id + 1;
}

void coverage_pre_call(thread_t* th)
{
	if (!flag_coverage)
		return;
	cover_reset(&th->cov);
	cover_reset(&th->rfcov);
}

void coverage_post_call(thread_t* th)
{
	if (!flag_coverage)
		return;
	cover_collect(&th->cov);
	if (th->cov.size >= kCoverSize)
		failmsg("too much cover", "thr=%d, cov=%u", th->id, th->cov.size);
	cover_collect(&th->rfcov);
	if (th->cov.size >= kCoverSize)
		failmsg("too much rf cover", "thr=%d, cov=%u", th->id, th->rfcov.size);
}

#if SYZ_EXECUTOR_USES_SHMEM
static uint32 hash(uint32 a)
{
	a = (a ^ 61) ^ (a >> 16);
	a = a + (a << 3);
	a = a ^ (a >> 4);
	a = a * 0x27d4eb2d;
	a = a ^ (a >> 15);
	return a;
}

const uint32 dedup_table_size = 8 << 10;
uint32 dedup_table[dedup_table_size];

// Poorman's best-effort hashmap-based deduplication.
// The hashmap is global which means that we deduplicate across different calls.
// This is OK because we are interested only in new signals.
static bool dedup(uint32 sig)
{
	for (uint32 i = 0; i < 4; i++) {
		uint32 pos = (sig + i) % dedup_table_size;
		if (dedup_table[pos] == sig)
			return true;
		if (dedup_table[pos] == 0) {
			dedup_table[pos] = sig;
			return false;
		}
	}
	dedup_table[sig % dedup_table_size] = sig;
	return false;
}
#endif // if SYZ_EXECUTOR_USES_SHMEM

template <typename T>
void copyin_int(char* addr, uint64 val, uint64 bf, uint64 bf_off, uint64 bf_len)
{
	if (bf_off == 0 && bf_len == 0) {
		*(T*)addr = swap(val, sizeof(T), bf);
		return;
	}
	T x = swap(*(T*)addr, sizeof(T), bf);
	debug_verbose("copyin_int<%zu>: old x=0x%llx\n", sizeof(T), (uint64)x);
#if __BYTE_ORDER__ == __ORDER_BIG_ENDIAN__
	const uint64 shift = sizeof(T) * CHAR_BIT - bf_off - bf_len;
#else
	const uint64 shift = bf_off;
#endif
	x = (x & ~BITMASK(shift, bf_len)) | ((val << shift) & BITMASK(shift, bf_len));
	debug_verbose("copyin_int<%zu>: new x=0x%llx\n", sizeof(T), (uint64)x);
	*(T*)addr = swap(x, sizeof(T), bf);
}

void copyin(char* addr, uint64 val, uint64 size, uint64 bf, uint64 bf_off, uint64 bf_len)
{
	debug_verbose("copyin: addr=%p val=0x%llx size=%llu bf=%llu bf_off=%llu bf_len=%llu\n",
		      addr, val, size, bf, bf_off, bf_len);
	if (bf != binary_format_native && bf != binary_format_bigendian && (bf_off != 0 || bf_len != 0))
		failmsg("bitmask for string format", "off=%llu, len=%llu", bf_off, bf_len);
	switch (bf) {
	case binary_format_native:
	case binary_format_bigendian:
		NONFAILING(switch (size) {
			case 1:
				copyin_int<uint8>(addr, val, bf, bf_off, bf_len);
				break;
			case 2:
				copyin_int<uint16>(addr, val, bf, bf_off, bf_len);
				break;
			case 4:
				copyin_int<uint32>(addr, val, bf, bf_off, bf_len);
				break;
			case 8:
				copyin_int<uint64>(addr, val, bf, bf_off, bf_len);
				break;
			default:
				failmsg("copyin: bad argument size", "size=%llu", size);
		});
		break;
	case binary_format_strdec:
		if (size != 20)
			failmsg("bad strdec size", "size=%llu", size);
		NONFAILING(sprintf((char*)addr, "%020llu", val));
		break;
	case binary_format_strhex:
		if (size != 18)
			failmsg("bad strhex size", "size=%llu", size);
		NONFAILING(sprintf((char*)addr, "0x%016llx", val));
		break;
	case binary_format_stroct:
		if (size != 23)
			failmsg("bad stroct size", "size=%llu", size);
		NONFAILING(sprintf((char*)addr, "%023llo", val));
		break;
	default:
		failmsg("unknown binary format", "format=%llu", bf);
	}
}

bool copyout(char* addr, uint64 size, uint64* res)
{
	return NONFAILING(
	    switch (size) {
		    case 1:
			    *res = *(uint8*)addr;
			    break;
		    case 2:
			    *res = *(uint16*)addr;
			    break;
		    case 4:
			    *res = *(uint32*)addr;
			    break;
		    case 8:
			    *res = *(uint64*)addr;
			    break;
		    default:
			    failmsg("copyout: bad argument size", "size=%llu", size);
	    });
}

uint64 read_arg(uint64** input_posp)
{
	uint64 typ = read_input(input_posp);
	switch (typ) {
	case arg_const: {
		uint64 size, bf, bf_off, bf_len;
		uint64 val = read_const_arg(input_posp, &size, &bf, &bf_off, &bf_len);
		if (bf != binary_format_native && bf != binary_format_bigendian)
			failmsg("bad argument binary format", "format=%llu", bf);
		if (bf_off != 0 || bf_len != 0)
			failmsg("bad argument bitfield", "off=%llu, len=%llu", bf_off, bf_len);
		return swap(val, size, bf);
	}
	case arg_result: {
		uint64 meta = read_input(input_posp);
		uint64 bf = meta >> 8;
		if (bf != binary_format_native)
			failmsg("bad result argument format", "format=%llu", bf);
		return read_result(input_posp);
	}
	default:
		failmsg("bad argument type", "type=%llu", typ);
	}
}

uint64 swap(uint64 v, uint64 size, uint64 bf)
{
	if (bf == binary_format_native)
		return v;
	if (bf != binary_format_bigendian)
		failmsg("bad binary format in swap", "format=%llu", bf);
	switch (size) {
	case 2:
		return htobe16(v);
	case 4:
		return htobe32(v);
	case 8:
		return htobe64(v);
	default:
		failmsg("bad big-endian int size", "size=%llu", size);
	}
}

uint64 read_const_arg(uint64** input_posp, uint64* size_p, uint64* bf_p, uint64* bf_off_p, uint64* bf_len_p)
{
	uint64 meta = read_input(input_posp);
	uint64 val = read_input(input_posp);
	*size_p = meta & 0xff;
	uint64 bf = (meta >> 8) & 0xff;
	*bf_off_p = (meta >> 16) & 0xff;
	*bf_len_p = (meta >> 24) & 0xff;
	uint64 pid_stride = meta >> 32;
	val += pid_stride * procid;
	*bf_p = bf;
	return val;
}

uint64 read_result(uint64** input_posp)
{
	uint64 idx = read_input(input_posp);
	uint64 op_div = read_input(input_posp);
	uint64 op_add = read_input(input_posp);
	uint64 arg = read_input(input_posp);
	if (idx >= kMaxCommands)
		failmsg("command refers to bad result", "result=%lld", idx);
	if (results[idx].executed) {
		arg = results[idx].val;
		if (op_div != 0)
			arg = arg / op_div;
		arg += op_add;
	}
	return arg;
}

uint64 read_input(uint64** input_posp, bool peek)
{
	uint64* input_pos = *input_posp;
	if ((char*)input_pos >= input_data + kMaxInput)
		failmsg("input command overflows input", "pos=%p: [%p:%p)", input_pos, input_data, input_data + kMaxInput);
	if (!peek)
		*input_posp = input_pos + 1;
	return *input_pos;
}

#if SYZ_EXECUTOR_USES_SHMEM
uint32* write_output(uint32 v)
{
	if (output_pos < output_data || (char*)output_pos >= (char*)output_data + output_size)
		failmsg("output overflow", "pos=%p region=[%p:%p]",
			output_pos, output_data, (char*)output_data + output_size);
	*output_pos = v;
	return output_pos++;
}

uint32* write_output_64(uint64 v)
{
	if (output_pos < output_data || (char*)(output_pos + 1) >= (char*)output_data + output_size)
		failmsg("output overflow", "pos=%p region=[%p:%p]",
			output_pos, output_data, (char*)output_data + output_size);
	*(uint64*)output_pos = v;
	output_pos += 2;
	return output_pos;
}

void write_completed(uint32 completed)
{
	__atomic_store_n(output_data, completed, __ATOMIC_RELEASE);
}
#endif // if SYZ_EXECUTOR_USES_SHMEM

#if SYZ_EXECUTOR_USES_SHMEM
void kcov_comparison_t::write()
{
	if (type > (KCOV_CMP_CONST | KCOV_CMP_SIZE_MASK))
		failmsg("invalid kcov comp type", "type=%llx", type);

	// Write order: type arg1 arg2 pc.
	write_output((uint32)type);

	// KCOV converts all arguments of size x first to uintx_t and then to
	// uint64. We want to properly extend signed values, e.g we want
	// int8 c = 0xfe to be represented as 0xfffffffffffffffe.
	// Note that uint8 c = 0xfe will be represented the same way.
	// This is ok because during hints processing we will anyways try
	// the value 0x00000000000000fe.
	switch (type & KCOV_CMP_SIZE_MASK) {
	case KCOV_CMP_SIZE1:
		arg1 = (uint64)(long long)(signed char)arg1;
		arg2 = (uint64)(long long)(signed char)arg2;
		break;
	case KCOV_CMP_SIZE2:
		arg1 = (uint64)(long long)(short)arg1;
		arg2 = (uint64)(long long)(short)arg2;
		break;
	case KCOV_CMP_SIZE4:
		arg1 = (uint64)(long long)(int)arg1;
		arg2 = (uint64)(long long)(int)arg2;
		break;
	}
	bool is_size_8 = (type & KCOV_CMP_SIZE_MASK) == KCOV_CMP_SIZE8;
	if (!is_size_8) {
		write_output((uint32)arg1);
		write_output((uint32)arg2);
	} else {
		write_output_64(arg1);
		write_output_64(arg2);
	}
}

bool kcov_comparison_t::ignore() const
{
	// Comparisons with 0 are not interesting, fuzzer should be able to guess 0's without help.
	if (arg1 == 0 && (arg2 == 0 || (type & KCOV_CMP_CONST)))
		return true;
	if ((type & KCOV_CMP_SIZE_MASK) == KCOV_CMP_SIZE8) {
		// This can be a pointer (assuming 64-bit kernel).
		// First of all, we want avert fuzzer from our output region.
		// Without this fuzzer manages to discover and corrupt it.
		uint64 out_start = (uint64)output_data;
		uint64 out_end = out_start + output_size;
		if (arg1 >= out_start && arg1 <= out_end)
			return true;
		if (arg2 >= out_start && arg2 <= out_end)
			return true;
#if defined(GOOS_linux)
		// Filter out kernel physical memory addresses.
		// These are internal kernel comparisons and should not be interesting.
		// The range covers first 1TB of physical mapping.
		uint64 kmem_start = (uint64)0xffff880000000000ull;
		uint64 kmem_end = (uint64)0xffff890000000000ull;
		bool kptr1 = arg1 >= kmem_start && arg1 <= kmem_end;
		bool kptr2 = arg2 >= kmem_start && arg2 <= kmem_end;
		if (kptr1 && kptr2)
			return true;
		if (kptr1 && arg2 == 0)
			return true;
		if (kptr2 && arg1 == 0)
			return true;
#endif
	}
	return !coverage_filter(pc);
}

bool kcov_comparison_t::operator==(const struct kcov_comparison_t& other) const
{
	// We don't check for PC equality now, because it is not used.
	return type == other.type && arg1 == other.arg1 && arg2 == other.arg2;
}

bool kcov_comparison_t::operator<(const struct kcov_comparison_t& other) const
{
	if (type != other.type)
		return type < other.type;
	if (arg1 != other.arg1)
		return arg1 < other.arg1;
	// We don't check for PC equality now, because it is not used.
	return arg2 < other.arg2;
}
#endif // if SYZ_EXECUTOR_USES_SHMEM

void setup_features(char** enable, int n)
{
	// This does any one-time setup for the requested features on the machine.
	// Note: this can be called multiple times and must be idempotent.
	flag_debug = true;
#if SYZ_HAVE_SETUP_EXT
	// This can be defined in common_ext.h.
	setup_ext();
#endif
#if SYZ_HAVE_FEATURES
	setup_sysctl();
	setup_cgroups();
#endif
	for (int i = 0; i < n; i++) {
		bool found = false;
#if SYZ_HAVE_FEATURES
		for (unsigned f = 0; f < sizeof(features) / sizeof(features[0]); f++) {
			if (strcmp(enable[i], features[f].name) == 0) {
				features[f].setup();
				found = true;
				break;
			}
		}
#endif
		if (!found)
			failmsg("setup features: unknown feature", "feature=%s", enable[i]);
	}
}

void setup_affinity_mask(int mask)
{
	cpu_set_t set;
	CPU_ZERO(&set);
	for (int i = 0; i < kMaxCPU; i++)
		if (mask & (1 << i))
			CPU_SET(i, &set);
	if (sched_setaffinity(0, sizeof(set), &set))
		fail("sched_setaffinity failed");
}

void failmsg(const char* err, const char* msg, ...)
{
	int e = errno;
	fprintf(stderr, "SYZFAIL: %s\n", err);
	if (msg) {
		va_list args;
		va_start(args, msg);
		vfprintf(stderr, msg, args);
		va_end(args);
	}
	fprintf(stderr, " (errno %d: %s)\n", e, strerror(e));

	// fail()'s are often used during the validation of kernel reactions to queries
	// that were issued by pseudo syscalls implementations. As fault injection may
	// cause the kernel not to succeed in handling these queries (e.g. socket writes
	// or reads may fail), this could ultimately lead to unwanted "lost connection to
	// test machine" crashes.
	// In order to avoid this and, on the other hand, to still have the ability to
	// signal a disastrous situation, the exit code of this function depends on the
	// current context.
	// All fail() invocations during system call execution with enabled fault injection
	// lead to termination with zero exit code. In all other cases, the exit code is
	// kFailStatus.
	if (current_thread && current_thread->soft_fail_state)
		doexit(0);
	doexit(kFailStatus);
}

void fail(const char* err)
{
	failmsg(err, 0);
}

void exitf(const char* msg, ...)
{
	int e = errno;
	va_list args;
	va_start(args, msg);
	vfprintf(stderr, msg, args);
	va_end(args);
	fprintf(stderr, " (errno %d)\n", e);
	doexit(0);
}

void debug_noprefix(const char* msg, ...)
{
	if (!flag_debug)
		return;
	int err = errno;
	va_list args;
	va_start(args, msg);
	vfprintf(stderr, msg, args);
	va_end(args);
	fflush(stderr);
	errno = err;
}

void debug(const char* msg, ...)
{
	if (!flag_debug)
		return;
	int err = errno;
	va_list args;
	fprintf(stderr, "[EXEC-%02lld/%02d] ", procid, threadid);
	va_start(args, msg);
	vfprintf(stderr, msg, args);
	va_end(args);
	fflush(stderr);
	errno = err;
}

void debug_dump_data(const char* data, int length)
{
	if (!flag_debug)
		return;
	int i = 0;
	fprintf(stderr, "[EXEC-%02lld/%02d] ", procid, threadid);
	for (; i < length; i++) {
		debug_noprefix("%02x ", data[i] & 0xff);
		if (i % 16 == 15) {
			debug_noprefix("\n");
			if (i != length - 1)
				debug_noprefix("             ");
		}
	}
	if (i % 16 != 0)
		debug_noprefix("\n");
}
