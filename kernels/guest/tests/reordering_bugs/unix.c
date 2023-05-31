#include <pthread.h>
#include <stdio.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <sys/un.h>

int sk;

#define SERVER_SOCK_FILE "unix_socket"
#define SIOCPROTOPRIVATE 0x89E0
#define SIOCUNIXFILE (SIOCPROTOPRIVATE + 0)

void *th1(void *unused) {
  struct sockaddr_un addr;
  memset(&addr, 0, sizeof(addr));
  addr.sun_family = AF_UNIX;
  strcpy(addr.sun_path, SERVER_SOCK_FILE);
  if (bind(sk, (struct sockaddr *)&addr, sizeof(addr)))
    perror("bind");
  return NULL;
}

void *th2(void *unused) {
  if (ioctl(sk, SIOCUNIXFILE, 0) < 0)
    perror("ioctl");
  return NULL;
}

int main() {
  sk = socket(AF_UNIX, SOCK_DGRAM, 0);
  printf("%d\n", sk);

  pthread_t pth1, pth2;

  pthread_create(&pth1, NULL, th1, NULL);
  pthread_create(&pth2, NULL, th2, NULL);

  pthread_join(pth1, NULL);
  pthread_join(pth2, NULL);

  return 0;
}
