// syz_mount_image$erofs(&(0x7f0000000180), &(0x7f00000001c0)='./file0\x00',
// 0x0, &(0x7f0000000200), 0x1, 0x17e,
// &(0x7f0000000240)="$eJzsmDFP8kAYx/935YW8xvgBXBwkEWMsbVHDwoCJu4mocZNIJWgBAzUREgc/gKODiYuDX8DRycFvoYM6uTA61/TuoCcR0EET4/MbHn5Hnx7Xh+Q/FARB/Fmen14fT5ezBQPAOJJIqO9fjKiHa/0P54dzZ7mVi+v7q7vaxPFN/34MQBB8/vdjAG7zBny1DoL3dyfVZwG85+vgmFW+CQZT+TY4NpS7YNhSvqd5Pew3zd2K55o7da8UihUWOyxOWDL95+ucMJS08zHterPV3i96ntv4Rhk1v06eI6edT/+/urOxtPnZ4LCVZ8CwpjyLRHc2ciTa80/Gov2NH35+EhKS3yZRPgWXDDNaPsW0/Ej71YN0s9Wer1SLZbfs1hwns2QtWNaikxZBJOuQ/Psv8mlM2//fgN44i+Oo6PsNW9be2pH1o8TlIv84UtNyHWZ/fOBp5HWm7mPCUsaQdoIgCIIgCIIgCIIgCIIgiC8wBSbego7AWRXdbwEAAP//gYN3SA==")
// open(&(0x7f0000000000)='./file1\x00', 0x0, 0x0)

#define _GNU_SOURCE

#include <errno.h>
#include <fcntl.h>
#include <linux/loop.h>
#include <pthread.h>
#include <setjmp.h> // for setjmp(), longjmp(), and jmp_buf
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/mount.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/xattr.h>
#include <syscall.h>
#include <unistd.h>

#define MAXBITS 15    // maximum bits in a code
#define MAXLCODES 286 // maximum number of literal/length codes
#define MAXDCODES 30  // maximum number of distance codes
#define MAXCODES (MAXLCODES + MAXDCODES) // maximum codes lengths to read
#define FIXLCODES 288                    // number of fixed literal/length codes

struct puff_state {
  // output state
  unsigned char *out;   // output buffer
  unsigned long outlen; // available space at out
  unsigned long outcnt; // bytes written to out so far

  // input state
  const unsigned char *in; // input buffer
  unsigned long inlen;     // available input at in
  unsigned long incnt;     // bytes read so far
  int bitbuf;              // bit buffer
  int bitcnt;              // number of bits in bit buffer

  // input limit error return state for bits() and decode()
  jmp_buf env;
};

// Return need bits from the input stream.  This always leaves less than
// eight bits in the buffer.  bits() works properly for need == 0.
static int puff_bits(struct puff_state *s, int need) {
  // bit accumulator (can use up to 20 bits)
  // load at least need bits into val
  long val = s->bitbuf;
  while (s->bitcnt < need) {
    if (s->incnt == s->inlen)
      longjmp(s->env, 1);                          // out of input
    val |= (long)(s->in[s->incnt++]) << s->bitcnt; // load eight bits
    s->bitcnt += 8;
  }

  // drop need bits and update buffer, always zero to seven bits left
  s->bitbuf = (int)(val >> need);
  s->bitcnt -= need;

  // return need bits, zeroing the bits above that
  return (int)(val & ((1L << need) - 1));
}

// Process a stored block.
static int puff_stored(struct puff_state *s) {
  // discard leftover bits from current byte (assumes s->bitcnt < 8)
  s->bitbuf = 0;
  s->bitcnt = 0;

  // get length and check against its one's complement
  if (s->incnt + 4 > s->inlen)
    return 2;                       // not enough input
  unsigned len = s->in[s->incnt++]; // length of stored block
  len |= s->in[s->incnt++] << 8;
  if (s->in[s->incnt++] != (~len & 0xff) ||
      s->in[s->incnt++] != ((~len >> 8) & 0xff))
    return -2; // didn't match complement!

  // copy len bytes from in to out
  if (s->incnt + len > s->inlen)
    return 2; // not enough input
  if (s->outcnt + len > s->outlen)
    return 1; // not enough output space
  for (; len--; s->outcnt++, s->incnt++) {
    if (s->in[s->incnt])
      s->out[s->outcnt] = s->in[s->incnt];
  }

  // done with a valid stored block
  return 0;
}

// Huffman code decoding tables.  count[1..MAXBITS] is the number of symbols of
// each length, which for a canonical code are stepped through in order.
// symbol[] are the symbol values in canonical order, where the number of
// entries is the sum of the counts in count[].  The decoding process can be
// seen in the function decode() below.
struct puff_huffman {
  short *count;  // number of symbols of each length
  short *symbol; // canonically ordered symbols
};

// Decode a code from the stream s using huffman table h.  Return the symbol or
// a negative value if there is an error.  If all of the lengths are zero, i.e.
// an empty code, or if the code is incomplete and an invalid code is received,
// then -10 is returned after reading MAXBITS bits.
static int puff_decode(struct puff_state *s, const struct puff_huffman *h) {
  int first = 0;          // first code of length len
  int index = 0;          // index of first code of length len in symbol table
  int bitbuf = s->bitbuf; // bits from stream
  int left = s->bitcnt;   // bits left in next or left to process
  int code = first = index = 0; // len bits being decoded
  int len = 1;                  // current number of bits in code
  short *next = h->count + 1;   // next number of codes
  while (1) {
    while (left--) {
      code |= bitbuf & 1;
      bitbuf >>= 1;
      int count = *next++;        // number of codes of length len
      if (code - count < first) { // if length len, return symbol
        s->bitbuf = bitbuf;
        s->bitcnt = (s->bitcnt - len) & 7;
        return h->symbol[index + (code - first)];
      }
      index += count; // else update for next length
      first += count;
      first <<= 1;
      code <<= 1;
      len++;
    }
    left = (MAXBITS + 1) - len;
    if (left == 0)
      break;
    if (s->incnt == s->inlen)
      longjmp(s->env, 1); // out of input
    bitbuf = s->in[s->incnt++];
    if (left > 8)
      left = 8;
  }
  return -10; // ran out of codes
}

// Given the list of code lengths length[0..n-1] representing a canonical
// Huffman code for n symbols, construct the tables required to decode those
// codes.  Those tables are the number of codes of each length, and the symbols
// sorted by length, retaining their original order within each length.  The
// return value is zero for a complete code set, negative for an over-
// subscribed code set, and positive for an incomplete code set.  The tables
// can be used if the return value is zero or positive, but they cannot be used
// if the return value is negative.  If the return value is zero, it is not
// possible for decode() using that table to return an error--any stream of
// enough bits will resolve to a symbol.  If the return value is positive, then
// it is possible for decode() using that table to return an error for received
// codes past the end of the incomplete lengths.

// Not used by decode(), but used for error checking, h->count[0] is the number
// of the n symbols not in the code.  So n - h->count[0] is the number of
// codes.  This is useful for checking for incomplete codes that have more than
// one symbol, which is an error in a dynamic block.

// Assumption: for all i in 0..n-1, 0 <= length[i] <= MAXBITS
// This is assured by the construction of the length arrays in dynamic() and
// fixed() and is not verified by construct().
static int puff_construct(struct puff_huffman *h, const short *length, int n) {
  // count number of codes of each length
  int len; // current length when stepping through h->count[]
  for (len = 0; len <= MAXBITS; len++)
    h->count[len] = 0;
  int symbol; // current symbol when stepping through length[]
  for (symbol = 0; symbol < n; symbol++)
    (h->count[length[symbol]])++; // assumes lengths are within bounds
  if (h->count[0] == n)           // no codes!
    return 0;                     // complete, but decode() will fail

  // check for an over-subscribed or incomplete set of lengths
  int left = 1; // one possible code of zero length
  for (len = 1; len <= MAXBITS; len++) {
    left <<= 1;            // one more bit, double codes left
    left -= h->count[len]; // deduct count from possible codes
    if (left < 0)
      return left; // over-subscribed--return negative
  }                // left > 0 means incomplete

  // generate offsets into symbol table for each length for sorting
  short offs[MAXBITS + 1];
  offs[1] = 0;
  for (len = 1; len < MAXBITS; len++)
    offs[len + 1] = offs[len] + h->count[len];

  // put symbols in table sorted by length, by symbol order within each length
  for (symbol = 0; symbol < n; symbol++)
    if (length[symbol] != 0)
      h->symbol[offs[length[symbol]]++] = symbol;

  // return zero for complete set, positive for incomplete set
  return left;
}

// Decode literal/length and distance codes until an end-of-block code.
static int puff_codes(struct puff_state *s, const struct puff_huffman *lencode,
                      const struct puff_huffman *distcode) {
  static const short lens[29] = {// Size base for length codes 257..285
                                 3,  4,  5,  6,   7,   8,   9,   10,  11, 13,
                                 15, 17, 19, 23,  27,  31,  35,  43,  51, 59,
                                 67, 83, 99, 115, 131, 163, 195, 227, 258};
  static const short lext[29] = {// Extra bits for length codes 257..285
                                 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2,
                                 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0};
  static const short dists[30] = {
      // Offset base for distance codes 0..29
      1,    2,    3,    4,    5,    7,    9,    13,    17,    25,
      33,   49,   65,   97,   129,  193,  257,  385,   513,   769,
      1025, 1537, 2049, 3073, 4097, 6145, 8193, 12289, 16385, 24577};
  static const short dext[30] = {// Extra bits for distance codes 0..29
                                 0, 0, 0,  0,  1,  1,  2,  2,  3,  3,
                                 4, 4, 5,  5,  6,  6,  7,  7,  8,  8,
                                 9, 9, 10, 10, 11, 11, 12, 12, 13, 13};

  // decode literals and length/distance pairs
  int symbol; // decoded symbol
  do {
    symbol = puff_decode(s, lencode);
    if (symbol < 0)
      return symbol;    // invalid symbol
    if (symbol < 256) { // literal: symbol is the byte
      // write out the literal
      if (s->outcnt == s->outlen)
        return 1;
      if (symbol)
        s->out[s->outcnt] = symbol;
      s->outcnt++;
    } else if (symbol > 256) { // length
      // get and compute length
      symbol -= 257;
      if (symbol >= 29)
        return -10; // invalid fixed code
      int len = lens[symbol] + puff_bits(s, lext[symbol]);

      // get and check distance
      symbol = puff_decode(s, distcode);
      if (symbol < 0)
        return symbol; // invalid symbol
      unsigned dist = dists[symbol] + puff_bits(s, dext[symbol]);
      if (dist > s->outcnt)
        return -11; // distance too far back

      // copy length bytes from distance bytes back
      if (s->outcnt + len > s->outlen)
        return 1;
      while (len--) {
        if (dist <= s->outcnt && s->out[s->outcnt - dist])
          s->out[s->outcnt] = s->out[s->outcnt - dist];
        s->outcnt++;
      }
    }
  } while (symbol != 256); // end of block symbol

  // done with a valid fixed or dynamic block
  return 0;
}

// Process a fixed codes block.
static int puff_fixed(struct puff_state *s) {
  static int virgin = 1;
  static short lencnt[MAXBITS + 1], lensym[FIXLCODES];
  static short distcnt[MAXBITS + 1], distsym[MAXDCODES];
  static struct puff_huffman lencode, distcode;

  // build fixed huffman tables if first call (may not be thread safe)
  if (virgin) {
    // construct lencode and distcode
    lencode.count = lencnt;
    lencode.symbol = lensym;
    distcode.count = distcnt;
    distcode.symbol = distsym;

    // literal/length table
    short lengths[FIXLCODES];
    int symbol;
    for (symbol = 0; symbol < 144; symbol++)
      lengths[symbol] = 8;
    for (; symbol < 256; symbol++)
      lengths[symbol] = 9;
    for (; symbol < 280; symbol++)
      lengths[symbol] = 7;
    for (; symbol < FIXLCODES; symbol++)
      lengths[symbol] = 8;
    puff_construct(&lencode, lengths, FIXLCODES);

    // distance table
    for (symbol = 0; symbol < MAXDCODES; symbol++)
      lengths[symbol] = 5;
    puff_construct(&distcode, lengths, MAXDCODES);

    // do this just once
    virgin = 0;
  }

  // decode data until end-of-block code
  return puff_codes(s, &lencode, &distcode);
}

// Process a dynamic codes block.
static int puff_dynamic(struct puff_state *s) {
  static const short order[19] = // permutation of code length codes
      {16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15};

  // get number of lengths in each table, check lengths
  int nlen = puff_bits(s, 5) + 257; // number of lengths in descriptor
  int ndist = puff_bits(s, 5) + 1;
  int ncode = puff_bits(s, 4) + 4;
  if (nlen > MAXLCODES || ndist > MAXDCODES)
    return -3; // bad counts

  // read code length code lengths (really), missing lengths are zero
  short lengths[MAXCODES]; // descriptor code lengths
  int index;               // index of lengths[]
  for (index = 0; index < ncode; index++)
    lengths[order[index]] = puff_bits(s, 3);
  for (; index < 19; index++)
    lengths[order[index]] = 0;

  // build huffman table for code lengths codes (use lencode temporarily)
  short lencnt[MAXBITS + 1], lensym[MAXLCODES];   // lencode memory
  struct puff_huffman lencode = {lencnt, lensym}; // length codes
  int err = puff_construct(&lencode, lengths, 19);
  if (err != 0) // require complete code set here
    return -4;

  // read length/literal and distance code length tables
  index = 0;
  while (index < nlen + ndist) {
    int symbol; // decoded value
    int len;    // last length to repeat

    symbol = puff_decode(s, &lencode);
    if (symbol < 0)
      return symbol; // invalid symbol
    if (symbol < 16) // length in 0..15
      lengths[index++] = symbol;
    else {                // repeat instruction
      len = 0;            // assume repeating zeros
      if (symbol == 16) { // repeat last length 3..6 times
        if (index == 0)
          return -5;              // no last length!
        len = lengths[index - 1]; // last length
        symbol = 3 + puff_bits(s, 2);
      } else if (symbol == 17) // repeat zero 3..10 times
        symbol = 3 + puff_bits(s, 3);
      else // == 18, repeat zero 11..138 times
        symbol = 11 + puff_bits(s, 7);
      if (index + symbol > nlen + ndist)
        return -6;     // too many lengths!
      while (symbol--) // repeat last or zero symbol times
        lengths[index++] = len;
    }
  }

  // check for end-of-block code -- there better be one!
  if (lengths[256] == 0)
    return -9;

  // build huffman table for literal/length codes
  err = puff_construct(&lencode, lengths, nlen);
  if (err && (err < 0 || nlen != lencode.count[0] + lencode.count[1]))
    return -7; // incomplete code ok only for single length 1 code

  // build huffman table for distance codes
  short distcnt[MAXBITS + 1], distsym[MAXDCODES];    // distcode memory
  struct puff_huffman distcode = {distcnt, distsym}; // distance codes
  err = puff_construct(&distcode, lengths + nlen, ndist);
  if (err && (err < 0 || ndist != distcode.count[0] + distcode.count[1]))
    return -8; // incomplete code ok only for single length 1 code

  // decode data until end-of-block code
  return puff_codes(s, &lencode, &distcode);
}

// Inflate source to dest.  On return, destlen and sourcelen are updated to the
// size of the uncompressed data and the size of the deflate data respectively.
// On success, the return value of puff() is zero.  If there is an error in the
// source data, i.e. it is not in the deflate format, then a negative value is
// returned.  If there is not enough input available or there is not enough
// output space, then a positive error is returned.  In that case, destlen and
// sourcelen are not updated to facilitate retrying from the beginning with the
// provision of more input data or more output space.  In the case of invalid
// inflate data (a negative error), the dest and source pointers are updated to
// facilitate the debugging of deflators.

// The return codes are:

//   2:  available inflate data did not terminate
//   1:  output space exhausted before completing inflate
//   0:  successful inflate
//  -1:  invalid block type (type == 3)
//  -2:  stored block length did not match one's complement
//  -3:  dynamic block code description: too many length or distance codes
//  -4:  dynamic block code description: code lengths codes incomplete
//  -5:  dynamic block code description: repeat lengths with no first length
//  -6:  dynamic block code description: repeat more than specified lengths
//  -7:  dynamic block code description: invalid literal/length code lengths
//  -8:  dynamic block code description: invalid distance code lengths
//  -9:  dynamic block code description: missing end-of-block code
// -10:  invalid literal/length or distance code in fixed or dynamic block
// -11:  distance is too far back in fixed or dynamic block
static int puff(unsigned char *dest,         // pointer to destination pointer
                unsigned long *destlen,      // amount of output space
                const unsigned char *source, // pointer to source data pointer
                unsigned long sourcelen)     // amount of input available
{
  struct puff_state s = {
      .out = dest,
      .outlen = *destlen,
      .outcnt = 0,
      .in = source,
      .inlen = sourcelen,
      .incnt = 0,
      .bitbuf = 0,
      .bitcnt = 0,
  };
  // return if bits() or decode() tries to read past available input
  int err;                // return value
  if (setjmp(s.env) != 0) // if came back here via longjmp()
    err = 2;              // then skip do-loop, return error
  else {
    // process blocks until last block or error
    int last;
    do {
      last = puff_bits(&s, 1);     // one if last block
      int type = puff_bits(&s, 2); // block type 0..3
      err = type == 0 ? puff_stored(&s)
                      : (type == 1 ? puff_fixed(&s)
                                   : (type == 2 ? puff_dynamic(&s)
                                                : -1)); // type == 3, invalid
      if (err != 0)
        break; // return with error
    } while (!last);
  }

  *destlen = s.outcnt;
  return err;
}

//% END CODE DERIVED FROM puff.{c,h}

#include <errno.h>
#include <sys/mman.h>
#define ZLIB_HEADER_WIDTH 2 // Two-byte zlib header width.

static int puff_zlib_to_file(const unsigned char *source,
                             unsigned long sourcelen, int dest_fd) {
  // Ignore zlib header.
  if (sourcelen < ZLIB_HEADER_WIDTH)
    return 0;
  source += ZLIB_HEADER_WIDTH;
  sourcelen -= ZLIB_HEADER_WIDTH;

  // Note: pkg/image/compression.go also knows this const.
  const unsigned long max_destlen = 132 << 20;
  void *ret = mmap(0, max_destlen, PROT_WRITE | PROT_READ,
                   MAP_PRIVATE | MAP_ANON, -1, 0);
  if (ret == MAP_FAILED)
    return -1;
  unsigned char *dest = (unsigned char *)ret;

  // Inflate source array to destination file.
  unsigned long destlen = max_destlen; // copy destlen as puff() may modify it
  int err = puff(dest, &destlen, source, sourcelen);
  if (err) {
    munmap(dest, max_destlen);
    errno = -err;
    return -1;
  }
  if (write(dest_fd, dest, destlen) != (ssize_t)destlen) {
    munmap(dest, max_destlen);
    return -1;
  }
  // Unmap memory-mapped region
  return munmap(dest, destlen);
}

#define debug printf
#define uint64 unsigned long long
#define procid 1

#define __NR_memfd_create 319

static int setup_loop_device(unsigned char *data, unsigned long size,
                             const char *loopname, int *loopfd_p) {
  int err = 0, loopfd = -1;
  int memfd = syscall(__NR_memfd_create, "syzkaller", 0);
  if (memfd == -1) {
    err = errno;
    goto error;
  }
  if (puff_zlib_to_file(data, size, memfd)) {
    err = errno;
    debug("setup_loop_device: could not decompress data: %d\n", errno);
    goto error_close_memfd;
  }

  loopfd = open(loopname, O_RDWR);
  if (loopfd == -1) {
    err = errno;
    goto error_close_memfd;
  }
  if (ioctl(loopfd, LOOP_SET_FD, memfd)) {
    if (errno != EBUSY) {
      err = errno;
      goto error_close_loop;
    }
    ioctl(loopfd, LOOP_CLR_FD, 0);
    usleep(1000);
    if (ioctl(loopfd, LOOP_SET_FD, memfd)) {
      err = errno;
      goto error_close_loop;
    }
  }

  close(memfd);
  *loopfd_p = loopfd;
  return 0;

error_close_loop:
  close(loopfd);
error_close_memfd:
  close(memfd);
error:
  errno = err;
  return -1;
}

static long syz_mount_image(volatile long fsarg, volatile long dir,
                            volatile long flags, volatile long optsarg,
                            volatile long change_dir,
                            volatile unsigned long size, volatile long image) {
  unsigned char *data = (unsigned char *)image;
  int res = -1, err = 0, loopfd = -1, need_loop_device = !!size;
  char *mount_opts = (char *)optsarg;
  char *target = (char *)dir;
  char *fs = (char *)fsarg;
  char *source = NULL;
  char loopname[64];

  if (need_loop_device) {
    // Some filesystems (e.g. FUSE) do not need a backing device or
    // filesystem image.
    memset(loopname, 0, sizeof(loopname));
    snprintf(loopname, sizeof(loopname), "/dev/loop%llu", procid);
    if (setup_loop_device(data, size, loopname, &loopfd) == -1)
      return -1;
    source = loopname;
  }

  mkdir(target, 0777);
  char opts[256];
  memset(opts, 0, sizeof(opts));
  // Leave some space for the additional options we append below.
  if (strlen(mount_opts) > (sizeof(opts) - 32)) {
    debug(
        "ERROR: syz_mount_image parameter optsarg bigger than internal opts\n");
  }
  strncpy(opts, mount_opts, sizeof(opts) - 32);
  if (strcmp(fs, "iso9660") == 0) {
    flags |= MS_RDONLY;
  } else if (strncmp(fs, "ext", 3) == 0) {
    // For ext2/3/4 we have to have errors=continue because the image
    // can contain errors=panic flag and can legally crash kernel.
    bool has_remount_ro = false;
    char *remount_ro_start = strstr(opts, "errors=remount-ro");
    if (remount_ro_start != NULL) {
      // syzkaller can sometimes break the options format, so we have to make
      // sure this option can really be parsed.
      char after = *(remount_ro_start + strlen("errors=remount-ro"));
      char before = remount_ro_start == opts ? '\0' : *(remount_ro_start - 1);
      has_remount_ro = ((before == '\0' || before == ',') &&
                        (after == '\0' || after == ','));
    }
    if (strstr(opts, "errors=panic") || !has_remount_ro)
      strcat(opts, ",errors=continue");
  } else if (strcmp(fs, "xfs") == 0) {
    // For xfs we need nouuid because xfs has a global uuids table
    // and if two parallel executors mounts fs with the same uuid, second mount
    // fails.
    strcat(opts, ",nouuid");
  }
  debug("syz_mount_image: size=%llu loop='%s' dir='%s' fs='%s' flags=%llu "
        "opts='%s'\n",
        (uint64)size, loopname, target, fs, (uint64)flags, opts);
#if SYZ_EXECUTOR
  cover_reset(0);
#endif
  res = mount(source, target, fs, flags, opts);
  if (res == -1) {
    debug("syz_mount_image > mount error: %d\n", errno);
    err = errno;
    goto error_clear_loop;
  }
  res = open(target, O_RDONLY | O_DIRECTORY);
  if (res == -1) {
    debug("syz_mount_image > open error: %d\n", errno);
    err = errno;
    goto error_clear_loop;
  }
  if (change_dir) {
    res = chdir(target);
    if (res == -1) {
      debug("syz_mount_image > chdir error: %d\n", errno);
      err = errno;
    }
  }

error_clear_loop:
  if (need_loop_device) {
    ioctl(loopfd, LOOP_CLR_FD, 0);
    close(loopfd);
  }
  errno = err;
  return res;
}

void *th0(void *unused) {
  if (open("./file0/file0", O_RDONLY, 0) < 0)
    perror("open");
  return NULL;
}

void *th1(void *unused) {
  char *buf = malloc(1024);
  listxattr("./file0/file0", buf, 1024);
  return NULL;
}

int main() {
  char *fsarg = "erofs";
  char *filename = "./file0";
  char *optsarg = "";
  char image[] = {
      120, 156, 236, 152, 49,  79,  242, 64,  24,  199, 255, 119, 229, 133, 188,
      198, 248, 1,   92,  28,  36,  17,  99,  44,  109, 81,  195, 194, 128, 137,
      187, 137, 168, 113, 147, 72,  37,  104, 1,   3,   53,  17,  18,  7,   63,
      128, 163, 131, 137, 139, 131, 95,  192, 209, 201, 193, 111, 161, 131, 58,
      185, 48,  58,  215, 244, 238, 160, 39,  17,  208, 65,  19,  227, 243, 27,
      30,  126, 71,  159, 30,  215, 135, 228, 63,  20,  4,   65,  252, 89,  158,
      159, 94,  31,  79,  151, 179, 5,   3,   192, 56,  146, 72,  168, 239, 95,
      140, 168, 135, 107, 253, 15,  231, 135, 115, 103, 185, 149, 139, 235, 251,
      171, 187, 218, 196, 241, 77,  255, 126, 12,  64,  16,  124, 254, 247, 99,
      0,   110, 243, 6,   124, 181, 14,  130, 247, 119, 39,  213, 103, 1,   188,
      231, 235, 224, 152, 85,  190, 9,   6,   83,  249, 54,  56,  54,  148, 187,
      96,  216, 82,  190, 167, 121, 61,  236, 55,  205, 221, 138, 231, 154, 59,
      117, 175, 20,  138, 21,  22,  59,  44,  78,  88,  50,  253, 231, 235, 156,
      48,  148, 180, 243, 49,  237, 122, 179, 213, 222, 47,  122, 158, 219, 248,
      70,  25,  53,  191, 78,  158, 35,  167, 157, 79,  255, 191, 186, 179, 177,
      180, 249, 217, 224, 176, 149, 103, 192, 176, 166, 60,  139, 68,  119, 54,
      114, 36,  218, 243, 79,  198, 162, 253, 141, 31,  126, 126, 18,  18,  146,
      223, 38,  81,  62,  5,   151, 12,  51,  90,  62,  197, 180, 252, 72,  251,
      213, 131, 116, 179, 213, 158, 175, 84,  139, 101, 183, 236, 214, 28,  39,
      179, 100, 45,  88,  214, 162, 147, 22,  65,  36,  235, 144, 252, 251, 47,
      242, 105, 76,  219, 255, 223, 128, 222, 56,  139, 227, 168, 232, 251, 13,
      91,  214, 222, 218, 145, 245, 163, 196, 229, 34,  255, 56,  82,  211, 114,
      29,  102, 127, 124, 224, 105, 228, 117, 166, 238, 99,  194, 82,  198, 144,
      118, 130, 32,  8,   130, 32,  8,   130, 32,  8,   130, 32,  8,   130, 32,
      136, 47,  48,  5,   38,  222, 130, 142, 192, 89,  21,  221, 111, 1,   0,
      0,   255, 255, 129, 131, 119};
  syz_mount_image((long)fsarg, (long)filename, 0, (long)optsarg, 1, 0x17e,
                  (long)image);
  pthread_t pth0, pth1;
  pthread_create(&pth0, NULL, th0, NULL);
  pthread_create(&pth1, NULL, th1, NULL);
  pthread_join(pth0, NULL);
  pthread_join(pth1, NULL);
  return 0;
}
