#include <cstdint>
#include <cstdlib>
#include <iostream>
#include <list>
#include <map>
#include <utility>

#define __DEBUG_SERIALIZE
#ifdef __DEBUG_SERIALIZE
#define DEBUG_PRINT(stmt) stmt
#else
#define DEBUG_PRINT(stmt)                                                      \
  do {                                                                         \
  } while (0)
#endif

class storebuffer {
public:
  storebuffer() = default;
  ~storebuffer() { flush(); }
  void store(uint64_t *addr, uint64_t val);
  uint64_t retrieve(uint64_t *addr);
  void flush();

private:
  // We maintain two store buffer copies, one for the load callback of
  // the calling thread itself, the other for the flush callback.
  // TODO: Seems we have redundant operations there.
  std::map<std::uint64_t *, uint64_t> _storebuffer_last_entry;
  std::list<std::pair<uint64_t *, uint64_t>> _storebuffer_indexed;
};

uint64_t storebuffer::retrieve(uint64_t *addr) {
  DEBUG_PRINT(std::string from);
  uint64_t val;
  if (_storebuffer_last_entry.find(addr) == _storebuffer_last_entry.end()) {
    // We don't have a thread local store buffer entry for
    // addr. Return the value in the global memory
    DEBUG_PRINT(from = "global memory");
    val = *addr;
  } else {
    DEBUG_PRINT(from = "store buffer");
    val = _storebuffer_last_entry[addr];
  }
  DEBUG_PRINT(std::cout << "Retreiving a value " << std::hex << val
                        << " from a " << from << " at " << std::hex << addr
                        << std::endl);
  return val;
}

void storebuffer::store(std::uint64_t *addr, const std::uint64_t val) {
  DEBUG_PRINT(std::cout << "Write a value " << std::hex << val
                        << " into a store buffer at " << std::hex << addr
                        << std::endl);
  _storebuffer_last_entry[addr] = val;
  _storebuffer_indexed.push_back(std::make_pair(addr, val));
}

void storebuffer::flush() {
  DEBUG_PRINT(std::cout << "flush all" << std::endl);
  for (auto const &entry : _storebuffer_indexed) {
    DEBUG_PRINT(std::cout << " flushing " << entry.second << std::hex
                          << " into " << (entry.first) << std::endl);
    *(entry.first) = entry.second;
  }
}

namespace {

thread_local storebuffer buffer;

std::uint64_t __load_callback_tso(std::uint64_t *addr, const std::size_t size) {
  return buffer.retrieve(addr);
}

void __store_callback_tso(std::uint64_t *addr, const std::uint64_t val,
                          const std::size_t size) {
  buffer.store(addr, val);
}

void __flush_callback_tso(const char *) { buffer.flush(); }

} // namespace

// #define __DEBUG_SERIALIZE

#include "runtime/decl_tso.h"
