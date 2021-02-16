#ifndef __CALLBACK_DECL_H
#error "Do not include _decl_callback_debug.h directly."
#endif

#ifndef __DECL_CALLBACK_DEBUG_H
#define __DECL_CALLBACK_DEBUG_H
// Debugging stuffs that serialize all execution of callback
// functions. It is safe to call any not-thread-safe functions (e.g.,
// printf, ...)
#define __STORE_CALLBACK_IMPL_ORIG STORE_CALLBACK_IMPL
#define __LOAD_CALLBACK_IMPL_ORIG LOAD_CALLBACK_IMPL

#include "runtime/_spinlock_dev.h"
spinlock _lock;
#define _LOCK() _lock.lock()
#define _UNLOCK() _lock.unlock()

#define ___DECLARE_STORE_CALLBACK_SERIALIZED(_MEMORYMODEL)                     \
  void __store_callback_##_MEMORYMODEL##_serialized(                           \
      std::uint64_t *addr, const std::uint64_t val, const std::size_t size) {  \
    _LOCK();                                                                   \
    __STORE_CALLBACK_IMPL_ORIG(addr, val, size);                               \
    _UNLOCK();                                                                 \
  }

#define ___DECLARE_LOAD_CALLBACK_SERIALIZED(_MEMORYMODEL)                      \
  std::uint64_t __load_callback_##_MEMORYMODEL##_serialized(                   \
      std::uint64_t *addr, const std::size_t size) {                           \
    std::uint64_t ret;                                                         \
    _LOCK();                                                                   \
    ret = __LOAD_CALLBACK_IMPL_ORIG(addr, size);                               \
    _UNLOCK();                                                                 \
    return ret;                                                                \
  }

#define __DECLARE_STORE_CALLBACK_SERIALIZED(_MEMORYMODEL)                      \
  ___DECLARE_STORE_CALLBACK_SERIALIZED(_MEMORYMODEL)
#define __DECLARE_LOAD_CALLBACK_SERIALIZED(_MEMORYMODEL)                       \
  ___DECLARE_LOAD_CALLBACK_SERIALIZED(_MEMORYMODEL)

#define STORE_CALLBACK_NAME_DEBUG(_MEMORYMODEL)                                \
  __store_callback_##_MEMORYMODEL##_serialized
#define LOAD_CALLBACK_NAME_DEBUG(_MEMORYMODEL)                                 \
  __load_callback_##_MEMORYMODEL##_serialized

__DECLARE_STORE_CALLBACK_SERIALIZED(MEMORYMODEL)
__DECLARE_LOAD_CALLBACK_SERIALIZED(MEMORYMODEL)

#undef _LOCK
#undef _UNLOCK

// Serialized callbacks are ready. Let's redefine
// STORE/LOAD_CALLBACK_IMPL macros
#undef STORE_CALLBACK_IMPL
#undef LOAD_CALLBACK_IMPL
#define __STORE_CALLBACK_IMPL(_MEMORYMODEL)                                    \
  STORE_CALLBACK_NAME_DEBUG(_MEMORYMODEL)
#define STORE_CALLBACK_IMPL __STORE_CALLBACK_IMPL(MEMORYMODEL)
#define __LOAD_CALLBACK_IMPL(_MEMORYMODEL)                                     \
  LOAD_CALLBACK_NAME_DEBUG(_MEMORYMODEL)
#define LOAD_CALLBACK_IMPL __LOAD_CALLBACK_IMPL(MEMORYMODEL)

#endif // __DECL_CALLBACK_DEBUG_H
