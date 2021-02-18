#include <cstdint>
#include <cstdlib>
#include <iostream>
#include <list>
#include <map>

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
  std::map<std::uint64_t *, std::list<uint64_t>> _storebuffer;
};

uint64_t storebuffer::retrieve(std::uint64_t *addr) {
  DEBUG_PRINT(std::string from);
  uint64_t val;
  if (_storebuffer[addr].size() == 0) {
    // This thread does not have an entry for addr. Read global memory
    DEBUG_PRINT(from = "global memory");
    val = *addr;
  } else {
    DEBUG_PRINT(from = "store buffer");
    val = _storebuffer[addr].back();
  }
  DEBUG_PRINT(std::cout << "Retreiving a value " << std::hex << val
                        << " from a " << from << " at " << std::hex << addr
                        << std::endl);
  return val;
}

void storebuffer::store(std::uint64_t *addr, std::uint64_t val) {
  DEBUG_PRINT(std::cout << "Write a value " << std::hex << val
                        << " into a store buffer at " << std::hex << addr
                        << std::endl);
  // TODO: Implement some memory init stuffs
  if (_storebuffer[addr].size() == 0)
    _storebuffer[addr].push_back(0);
  else
    _storebuffer[addr].push_back(val);
}

void storebuffer::flush() {
  // In PSO, we does not need to flush store buffer entires in order
  // of stores
  DEBUG_PRINT(std::cout << "flush all" << std::endl);
  for (auto const &entry : _storebuffer) {
    // TODO: Is it okay to flush the last value?
    for (auto const val : entry.second) {
      DEBUG_PRINT(std::cout << " flushing " << val << std::hex << " into "
                            << (entry.first) << std::endl);
      *(entry.first) = val;
    }
  }
}

namespace {

thread_local storebuffer buffer;

std::uint64_t __load_callback_pso(std::uint64_t *addr, const std::size_t size) {
  uint64_t val = buffer.retrieve(addr);
  return val;
}

void __store_callback_pso(std::uint64_t *addr, const std::uint64_t val,
                          const std::size_t size) {
  buffer.store(addr, val);
}

void __flush_callback_pso(char *) { buffer.flush(); }

} // namespace

#include "runtime/decl_pso.h"
