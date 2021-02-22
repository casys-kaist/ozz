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

#include "runtime/input_vector.h"
inputVector input;

class Entry final {
public:
  Entry(uint64_t _value, size_t _size) : value(_value), size(_size) {}
  uint64_t value;
  size_t size;
};

class storebuffer {
public:
  storebuffer() = default;
  ~storebuffer() { flushAllEntries(); }
  void store(uint64_t *addr, uint64_t val, size_t size);
  uint64_t retrieve(uint64_t *addr, size_t size);
  void flushEntry(uint64_t *addr, const Entry entry);
  void flushEntries(const int n);
  void flushAllEntries();
  void flushAfterInsn();

private:
  // We maintain two store buffer copies, one for the load callback of
  // the calling thread itself, the other for the flush callback.
  // TODO: Seems we have redundant operations there.

  // Byte-level entry
  std::map<char *, uint8_t> _storebuffer_last_entry;
  std::list<std::pair<uint64_t *, Entry>> _storebuffer_indexed;
#define kMaxBufferSize 16
  int bufferSize;
};

uint64_t storebuffer::retrieve(uint64_t *_addr, size_t size) {
  uint64_t val;
  char *valByte = (char *)&val;

  DEBUG_PRINT(std::cout << "Retreiving a value " << std::hex << _addr
                        << std::endl);

  for (unsigned int i = 0; i < size; i++) {
    char *addr = (char *)_addr + i;
    if (_storebuffer_last_entry.find(addr) == _storebuffer_last_entry.end()) {
      // We don't have a thread local store buffer entry for
      // addr. Return the value in the global memory
      *(valByte + i) = *addr;
      DEBUG_PRINT(std::cout << "  <-" << std::hex << (void *)addr << std::endl);
    } else {
      *(valByte + i) = _storebuffer_last_entry[addr];
    }
  }

  DEBUG_PRINT(std::cout << "  ->" << val << std::endl);

  flushAfterInsn();

  return val;
}

void storebuffer::store(std::uint64_t *addr, const std::uint64_t val,
                        const size_t size) {
  DEBUG_PRINT(std::cout << "Write a value " << std::hex << val
                        << " into a store buffer at " << std::hex << addr
                        << std::endl);
  // _storebuffer_last_entry[addr] = entry;
  char *valByte = (char *)&val;
  for (unsigned int i = 0; i < size; i++)
    _storebuffer_last_entry[(char *)addr + i] = *(char *)&(valByte[i]);
  // Will be used later when flushing
  Entry entry(val, size);
  _storebuffer_indexed.push_back(std::make_pair(addr, entry));
  bufferSize++;
  flushAfterInsn();
}

void storebuffer::flushAfterInsn() {
  if (bufferSize >= kMaxBufferSize)
    flushAllEntries();
  else if (input.size() > 0)
    flushEntries(input.next());
}

void storebuffer::flushEntry(uint64_t *addr, const Entry entry) {
  DEBUG_PRINT(std::cout << " flushing " << entry.value << std::hex << " into "
                        << addr << std::endl);
  switch (entry.size) {
  case 1: {
    uint8_t val = (char)entry.value;
    *(uint8_t *)addr = val;
  } break;
  case 2: {
    uint16_t val = (char)entry.value;
    *(uint16_t *)addr = val;
  } break;
  case 4: {
    uint32_t val = (char)entry.value;
    *(uint32_t *)addr = val;
  }; break;
  case 8:
    *addr = entry.value;
    break;
  default:;
  }
}

void storebuffer::flushEntries(const int n) {
  DEBUG_PRINT(std::cout << "Flush " << n << std::endl);
  for (int i = 0; i < n && _storebuffer_indexed.size(); i++) {
    auto _entry = _storebuffer_indexed.front();
    char *base = (char *)_entry.first;
    for (unsigned int j = 0; j < _entry.second.size; j++)
      _storebuffer_last_entry.erase(base + j);
    flushEntry(_entry.first, _entry.second);
    // Remove
    _storebuffer_indexed.pop_front();
  }
}

void storebuffer::flushAllEntries() {
  DEBUG_PRINT(std::cout << "Flush all" << std::endl);
  for (auto const &_entry : _storebuffer_indexed)
    flushEntry(_entry.first, _entry.second);

  _storebuffer_last_entry.clear();
  bufferSize = 0;
}

namespace {

thread_local storebuffer buffer;

std::uint64_t __load_callback_tso(std::uint64_t *addr, const std::size_t size) {
  return buffer.retrieve(addr, size);
}

void __store_callback_tso(std::uint64_t *addr, const std::uint64_t val,
                          const std::size_t size) {
  buffer.store(addr, val, size);
}

void __flush_callback_tso(const char *) { buffer.flushAllEntries(); }

void __feedinput_callback_tso(std::uint32_t _input[], const std::size_t size) {
  input.feedInput(_input, size);
}

} // namespace

// #define __DEBUG_SERIALIZE

#include "runtime/decl_tso.h"
