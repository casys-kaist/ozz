# tests

The `tests` directory contains various tests for our implementaions.

## `pso_tests.c`

testing that kssb can trigger a simple synthetic bug. thread scheduling is hard-coded.

## `qcsched_test.c`

testing qcsched's mechanisms. it does not trigger a bug in the kernel.

## `integration_test.c`

testing that kssb and qcsched can trigger a simple synthetic bug together.

## `kmemcov_test.c`

testing that kmemcov works. it does not check whether the result is correct or not

## `sbitmap_integration_test.c`

testing that kssb and qcsched can trigger a complex synthetic bug.

## `blk-mq_real-world.c`

testing that kssb and qcsched can trigger a (slightly modified) real-world bug.
