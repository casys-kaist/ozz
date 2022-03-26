all:
	mkdir -p out/
	$(CC) test.c -lpthread -o out/qcsched_test-cve-2019-6974 -DCVE20196974 -DTEST_REPEAT -DTEST_KMEMCOV
	$(CC) test.c -lpthread -o out/qcsched_test-cve-2019-6974-minimal -DCVE20196974_MINIMAL
	$(CC) test.c -lpthread -o out/qcsched_test-simple -DSIMPLE_TEST
	$(CC) test.c -lpthread -o out/qcsched_test-bypass -DBYPASS_TEST
	$(CC) test.c -lpthread -o out/qcsched_test-spinlock -DSPINLOCK_TEST -DTEST_KMEMCOV
	$(CC) test.c -lpthread -o out/qcsched_test-footprint -DFOOTPRINT_TEST
	$(CC) test.c -lpthread -o out/qcsched_test-bypass-repeat -DBYPASS_TEST -DTEST_REPEAT

clean:
	rm -rf out/

.PHONY: all clean
