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

#include "runtime/input_vector.h"
inputVector input;

class storebuffer {
public:
  storebuffer() = default;
  ~storebuffer() { flushAll(); }
  void store(uint64_t *addr, uint64_t val);
  uint64_t retrieve(uint64_t *addr);
  void flush(uint64_t *addr, const int n);
  void flushAll();
  void flushAfterInsn(uint64_t *addr);

private:
#define kMaxBufferSize 16
  int bufferSize;
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
  flushAfterInsn(addr);
  return val;
}

void storebuffer::store(std::uint64_t *addr, std::uint64_t val) {
  DEBUG_PRINT(std::cout << "Write a value " << std::hex << val
                        << " into a store buffer at " << std::hex << addr
                        << std::endl);
  _storebuffer[addr].push_back(val);
  bufferSize++;
  flushAfterInsn(addr);
}

void storebuffer::flush(uint64_t *addr, const int n) {
  if (_storebuffer[addr].empty())
    return;
  DEBUG_PRINT(std::cout << "Flush " << n << " at " << std::hex << addr
                        << std::endl);
  for (int i = 0; i < n && !_storebuffer[addr].empty(); i++) {
    auto val = _storebuffer[addr].front();
    DEBUG_PRINT(std::cout << " flushing " << val << std::endl);
    *(addr) = val;
    _storebuffer[addr].pop_front();
  }
}

void storebuffer::flushAll() {
  // In PSO, we does not need to flush store buffer entires in order
  // of stores
  DEBUG_PRINT(std::cout << "Flush all" << std::endl);
  for (auto const &entry : _storebuffer) {
    // TODO: Is it okay to flush the last value?
    for (auto const val : entry.second) {
      DEBUG_PRINT(std::cout << " flushing " << val << std::hex << " into "
                            << (entry.first) << std::endl);
      *(entry.first) = val;
    }
  }
  _storebuffer.clear();
  bufferSize = 0;
}

void storebuffer::flushAfterInsn(uint64_t *addr) {
  if (bufferSize >= kMaxBufferSize)
    flushAll();
  else if (input.size() > 0)
    flush(addr, input.next());
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

void __flush_callback_pso(char *) { buffer.flushAll(); }

void __feedinput_callback_pso(std::uint32_t _input[], const std::size_t size) {
  input.feedInput(_input, size);
}

} // namespace

#include "runtime/decl_pso.h"
