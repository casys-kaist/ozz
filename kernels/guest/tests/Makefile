OBJ=pso_test
all: pso_test.c
	@gcc $^ -o $(OBJ)-x86_64 -lpthread
	@aarch64-linux-gnu-gcc $^ -o $(OBJ)-aarch64 -lpthread

clean:
	@rm $(OBJ)-x86_64
	@rm $(OBJ)-aarch64

.PHONY: all clean
