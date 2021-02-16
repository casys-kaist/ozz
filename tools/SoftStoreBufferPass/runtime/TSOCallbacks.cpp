#include <cstdint>
#include <cstdlib>

namespace {

std::uint64_t __load_callback_tso(std::uint64_t *addr, const std::size_t size) {
  return *addr;
}

void __store_callback_tso(std::uint64_t *addr, const std::uint64_t val,
                          const std::size_t size) {
  *addr = val;
}

} // namespace

// #define __DEBUG_SERIALIZE

#include "runtime/decl_tso.h"
