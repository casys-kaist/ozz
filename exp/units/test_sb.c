/*
 * Count portion of reordering in store buffering model.
 */

#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <stdatomic.h>
#include "test.h"

#define NUM_TEST 1000000
#define CACHELINE_SIZE 64

struct shared_t {
    int f1;
    char _pad[60];
    int f2;
    int r1;
    int r2;
};

atomic_int ready = -2;
atomic_int start = -1;
atomic_int done = -2;

void *th1(void *aux) {
    pin(1);

    int r2;
    struct shared_t *shared = ((struct shared_t *)aux);

    for (int i = 0; i < NUM_TEST; i++) {

        r2 = 0;

        ready++;
        __sync_synchronize();

        while (i != start) ;

        shared[i].f1 = 1;
        r2 = shared[i].f2;

        shared[i].r2 = r2;

        __sync_synchronize();
        done++;
    }

    return NULL;
}

void *th2(void *aux) {
    pin(2);

    int r1;
    struct shared_t *shared = ((struct shared_t *)aux);

    for (int i = 0; i < NUM_TEST; i++) {

        r1 = 0;

        ready++;
        __sync_synchronize();

        while (i != start) ;

        shared[i].f2 = 1;
        r1 = shared[i].f1;

        shared[i].r1 = r1;

        __sync_synchronize();
        done++;
    }

    return NULL;
}

int main() {
    int cnt = 0;

    pin(0);

    pthread_t pth1, pth2;
    struct shared_t * shared_arr = calloc(NUM_TEST, sizeof(struct shared_t));

    pthread_create(&pth1, NULL, th1, shared_arr);
    pthread_create(&pth2, NULL, th2, shared_arr);

    for (int i = 0; i < NUM_TEST; i++) {
        while(2*i != ready) ;
        __sync_synchronize();
        start++;
        __sync_synchronize();
        while(2*i != done) ;
    }

    printf("aligns: %p %p\n", &shared_arr[0].f1, &shared_arr[0].f2);

    pthread_join(pth1, NULL);
    pthread_join(pth2, NULL);

    for (int i = 0; i < NUM_TEST; i++)
        if (shared_arr[i].r1 == 0 && shared_arr[i].r2 == 0) cnt++;
    
    printf("cnt: %d/%d\n", cnt, NUM_TEST);

    return 0;
}