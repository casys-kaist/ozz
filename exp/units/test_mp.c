/*
 * Count portion of reordering in message passing model.
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

// buf and flag should be cacheline-aligned
struct shared_t {
    int buf;
    char _pad0[60];
    int flag;
    int r1;
    int r2;
    char _pad1[5];
};

atomic_int ready = -2;
atomic_int start = -1;
atomic_int done = -2;

void *th1(void *aux) {
    struct shared_t *shared = ((struct shared_t *)aux);

    pin(1);

    for (int i = 0; i < NUM_TEST; i++) {

        ready++;
        __sync_synchronize();

        while (i != start) ;

        shared[i].buf = 1;
        shared[i].flag = 1;

        __sync_synchronize();
        done++;
    }

    return NULL;
}

void *th2(void *aux) {
    int r1, r2;
    struct shared_t *shared = ((struct shared_t *)aux);

    pin(2);

    for (int i = 0; i < NUM_TEST; i++) {

        r1 = 0;
        r2 = 0;

        ready++;
        __sync_synchronize();

        while (i != start) ;

        r1 = shared[i].flag;
        r2 = shared[i].buf;

        shared[i].r1 = r1;
        shared[i].r2 = r2;

        __sync_synchronize();
        done++;
    }

    return NULL;
}

int main() {
    int cnt = 0;
    struct shared_t *shared_arr = calloc(NUM_TEST+1, sizeof(struct shared_t));
    
    // Align starting address to cacheline
    struct shared_t *shared_arr_aligned = 
        (struct shared_t *) (((char *) shared_arr) - ((unsigned long) shared_arr % CACHELINE_SIZE) + CACHELINE_SIZE);

    printf("size: 0x%lx aligns: %p %p\n", sizeof(struct shared_t), 
        &shared_arr_aligned[0].buf, &shared_arr_aligned[0].flag);

    pin(0);

    pthread_t pth1, pth2;

    pthread_create(&pth1, NULL, th1, shared_arr_aligned);
    pthread_create(&pth2, NULL, th2, shared_arr_aligned);

    for (int i = 0; i < NUM_TEST; i++) {
        while(2*i != ready) ;
        __sync_synchronize();
        start++;
        __sync_synchronize();
        while(2*i != done) ;
    }

    pthread_join(pth1, NULL);
    pthread_join(pth2, NULL);

    // Count number of reordered cases 
    for (int i = 0; i < NUM_TEST; i++)
        if (shared_arr_aligned[i].r1 == 1 && shared_arr_aligned[i].r2 == 0) cnt++;

    free(shared_arr);

    printf("cnt: %d/%d\n", cnt, NUM_TEST);

    return 0;
}
