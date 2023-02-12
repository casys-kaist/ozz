#!/usr/bin/env python

import random
total = 0
for i in range(10):
    arr = list(range(323))
    cnt = 0
    while True:
        idx = random.randint(0, len(arr)-1)
        cnt += 1
        if idx >= 0 and idx <= 2:
            break
        arr.pop(idx)
    
    print(cnt)
    total += cnt

print(total)
