#!/usr/bin/env python

import sys

GOLDEN_RATIO = 0x61C8864680B583EB
UINT64_MAX = 0xFFFFFFFFFFFFFFF


def hash64(val, bits):
    return ((val * GOLDEN_RATIO) % (UINT64_MAX + 1)) >> (64 - bits)


def main():
    import argparse

    parser = argparse.ArgumentParser()
    parser.add_argument("--bits", action="store", type=int, default=64)
    parser.add_argument("--mod", action="store", type=int, default=UINT64_MAX)
    parser.add_argument("inst", nargs="+")
    args = parser.parse_args()

    for arg in args.inst:
        inst = int(arg, 0)
        print(hash64(inst, args.bits) % args.mod)


if __name__ == "__main__":
    main()
