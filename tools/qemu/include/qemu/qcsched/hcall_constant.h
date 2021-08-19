#ifndef __HCALL_CONSTANT_H
#define __HCALL_CONSTANT_H

// RAX input value of hcall request
#define HCALL_RAX_ID 0x1d08aa3e
// RAX return value indicating a hcall handled successfully
#define HCALL_SUCCESS 0x2be98adc
// RAX return value indicating a bad request
#define HCALL_INVAL 0xb614e7a

// kvm_run->exit_reason
#define HCALL_EXIT_REASON 0x33f355d
#define KVM_EXIT_HCALL HCALL_EXIT_REASON

// Commands saved in kvm_run->hypercall.args[0]
#define HCALL_INSTALL_BP 0xf477909a
#define HCALL_ACTIVATE_BP 0x40ab903
#define HCALL_DEACTIVATE_BP 0xf327524f
#define HCALL_CLEAR_BP 0xba220681
#define HCALL_VMI_HINT 0x7ca889f0

// Subcommands for HCALL_VMI_HINT (saved in kvm_run->hypercall.args[1])
#define VMI_TRAMPOLINE 0x939aef52
#define VMI_HOOK 0x30f4b16
#define VMI_CURRENT_TASK 0xfb40de5
#define VMI__PER_CPU_OFFSET0 0x4a157131
#define VMI__SSB_DO_EMULATE 0xdb17901

#endif /* __HCALL_CONSTANT_H */
