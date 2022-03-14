all:
	mkdir -p out/
	$(CC) cve-2019-6974.c -lpthread -o out/qcsched_test-cve-2019-6974 -DCVE20196974
	$(CC) cve-2019-6974.c -lpthread -o out/qcsched_test-simple -DSIMPLE_TEST
	$(CC) cve-2019-6974.c -lpthread -o out/qcsched_test-bypass -DBYPASS_TEST

clean:
	rm -rf out/

.PHONY: all clean
