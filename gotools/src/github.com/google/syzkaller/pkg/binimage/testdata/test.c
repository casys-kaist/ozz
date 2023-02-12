#include <stdio.h>

int a = 1;

void foo() {
	printf("%d", a);
	a = 2;
	printf("%d", a);
}

void bar() {
	a = 0;
	foo();
}

int main(void) {
	foo();
	bar();
	return 0;
}
