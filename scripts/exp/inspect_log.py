#!/usr/bin/env python

import argparse
import sys


def inspect_log(lines, time_threshold_s):
    import datetime
    import re

    threshold = datetime.timedelta(seconds=time_threshold_s)

    prev = None
    for line in lines:
        time_strs = re.search("\[[^\[\]]*\]", line)
        if time_strs == None:
            continue
        time_str = time_strs[0][1:-1]
        time_obj = datetime.datetime.strptime(time_str, "%Y-%m-%d %H:%M:%S.%f")
        if prev != None and time_obj - prev > threshold:
            print("-----------------------------------------------------------")
        print(line.strip())
        prev = time_obj


def main():
    parser = argparse.ArgumentParser(description="inspect fuzzer's log")
    parser.add_argument(
        "filename",
        type=str,
        help="name of the log file",
    )
    parser.add_argument("--time-threshold", type=int, default=1)

    args = parser.parse_args()

    with open(args.filename) as f:
        lines = f.readlines()

    inspect_log(lines, args.time_threshold)


if __name__ == "__main__":
    main()
