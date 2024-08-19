#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <stdio.h>
#include <unistd.h>

#include "hypercall.h"
#include "test.h"

void run() {
    hypercall(HCALL_DISABLE_KSSB, 0, 0, 0);
}

int main(void) {
  run();
  return 0;
}
