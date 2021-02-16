// Ref: https://gist.github.com/vertextao/9a9077720c15fec89ed1f3fd91c9e91a

// Spinlock implementation used during
// development/debugging. Shamelessly borrowed from vertextao.

#ifndef __SPINLOCK_DEV_H
#define __SPINLOCK_DEV_H

#include <atomic>

class spinlock {
public:
  spinlock() { m_lock.clear(); }
  spinlock(const spinlock &) = delete;
  ~spinlock() = default;

  void lock() {
    while (m_lock.test_and_set(std::memory_order_acquire))
      ;
  }
  bool try_lock() { return !m_lock.test_and_set(std::memory_order_acquire); }
  void unlock() { m_lock.clear(std::memory_order_release); }

private:
  std::atomic_flag m_lock;
};

#endif // __SPINLOCK_H
