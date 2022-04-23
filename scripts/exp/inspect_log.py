#!/usr/bin/env python

import argparse
import re
import sys

work_types = [
    ("executing a candidate", "candidate"),
    ("triaging type=", "triage"),
    ("generated", "generate"),
    ("mutated", "mutate"),
    ("scheduling an input", "schedule"),
    ("smash mutated", "smash"),
    ("threading an input", "thread"),
]

count = {}


def work_type(line):
    if re.search("proc #[0-9]", line) == None:
        return None
    for typ in work_types:
        if line.find(typ[0]) != -1:
            return typ[1]


def inspect_log(lines, time_threshold_s):
    import datetime

    threshold = datetime.timedelta(seconds=time_threshold_s)

    first, last = None, None
    prev = None

    for line in lines:
        line = line.strip()
        time_strs = re.search("\[[^\[\]]*\]", line)
        if time_strs == None:
            continue
        time_str = time_strs[0][1:-1]
        time_obj = datetime.datetime.strptime(time_str, "%Y-%m-%d %H:%M:%S.%f")
        if prev != None and time_obj - prev > threshold:
            print("-----------------------------------------------------------")
        print(line)
        prev = time_obj
        if first == None:
            first = time_obj
        last = time_obj
        typ = work_type(line)
        if typ == None:
            continue
        count[typ] = 1 if typ not in count else count[typ] + 1

    print("Total time = ", last - first)
    print(count)


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
