#ifndef MEMORYMODEL
#error "Memory model is not defined"
#endif

#ifndef STORE_CALLBACK_IMPL
#error "Store callback is not defined"
#endif

#ifndef LOAD_CALLBACK_IMPL
#error "Load callback is not defined"
#endif

#ifndef FLUSH_CALLBACK_IMPL
#error "Flush callback is not defined"
#endif

#ifdef __CALLBACK_DECL_H
// Since this header file defines callback functions, we should not
// include this multiple times
#error "callback_decl.h is included multiple times"
#endif // __CALLBACK_DECL_H

#define __CALLBACK_DECL_H

#ifdef __DEBUG_SERIALIZE
#include "runtime/_decl_callback_debug.h"
#endif // __DEBUG_SERIALIZE

#include <cstdint>

extern "C" {

// TODO: Seems really ugly. Replace this with any better way
#define _BYTE_1_TO_BITS 8
#define _BYTE_2_TO_BITS 16
#define _BYTE_4_TO_BITS 32
#define _BYTE_8_TO_BITS 64
#define _BYTE_16_TO_BITS 128

#define BIT_MASK(_BITS) (_BITS == 64 ? 0xffffffffffffffff : (1ULL << _BITS) - 1)

#define __DEFINE_STORE_CALLBACK(_MEMORYMODEL, _BYTES, _BITS)                   \
  void __ssb_##_MEMORYMODEL##_store##_BYTES(char *addr,                        \
                                            std::uint##_BITS##_t val)

// The val argument (typed uintN_t) will be promoted to uint64_t
#define __DECLARE_STORE_CALLBACK(_MEMORYMODEL, _BYTES, _BITS)                  \
  __DEFINE_STORE_CALLBACK(_MEMORYMODEL, _BYTES, _BITS) {                       \
    uint64_t _val = (uint64_t)val & BIT_MASK(_BITS);                           \
    STORE_CALLBACK_IMPL((std::uint64_t *)addr, _val, _BYTES);                  \
  }

#define __DEFINE_LOAD_CALLBACK(_MEMORYMODEL, _BYTES, _BITS)                    \
  std::uint##_BITS##_t __ssb_##_MEMORYMODEL##_load##_BYTES(char *addr)

// The return value of LOAD_CALLBACK_IMPL (typed uint64_t) will be
// demoted to uintN_t
#define __DECLARE_LOAD_CALLBACK(_MEMORYMODEL, _BYTES, _BITS)                   \
  __DEFINE_LOAD_CALLBACK(_MEMORYMODEL, _BYTES, _BITS) {                        \
    std::uint##_BITS##_t val =                                                 \
        LOAD_CALLBACK_IMPL((std::uint64_t *)addr, _BYTES);                     \
    uint##_BITS##_t _val = (uint##_BITS##_t)(val & BIT_MASK(_BITS));           \
    return _val;                                                               \
  }

#define __DEFINE_FLUSH_CALLBACK(_MEMORYMODEL)                                  \
  void __ssb_##_MEMORYMODEL##_flush(char *addr)

#define DECLARE_FLUSH_CALLBACK(_MEMORYMODEL)                                   \
  __DEFINE_FLUSH_CALLBACK(_MEMORYMODEL) { FLUSH_CALLBACK_IMPL(addr); }

#define __DECLARE_STORE_LOAD_CALLBACK(_MEMORYMODEL, _BYTES, _BITS)             \
  __DECLARE_STORE_CALLBACK(_MEMORYMODEL, _BYTES, _BITS)                        \
  __DECLARE_LOAD_CALLBACK(_MEMORYMODEL, _BYTES, _BITS)

#define DECLARE_STORE_LOAD_CALLBACK(_BYTES)                                    \
  __DECLARE_STORE_LOAD_CALLBACK(MEMORYMODEL, _BYTES, _BYTE_##_BYTES##_TO_BITS)

DECLARE_STORE_LOAD_CALLBACK(1)
DECLARE_STORE_LOAD_CALLBACK(2)
DECLARE_STORE_LOAD_CALLBACK(4)
#ifdef UINT64_MAX
DECLARE_STORE_LOAD_CALLBACK(8)
#endif
#ifdef UINT128_MAX
DECLARE_STORE_LOAD_CALLBACK(16)
#endif
DECLARE_FLUSH_CALLBACK(MEMORYMODEL)

} // extern "C"
