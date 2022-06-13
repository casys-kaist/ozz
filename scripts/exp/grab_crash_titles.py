#!/usr/bin/env python

"""script to grap all titles from machines"""

import argparse
import json
import os
import subprocess


def grab(machine):
    path = os.path.join(machine["workdir"])
    find_cmd = 'find {} -name description -printf "%h " -exec cat {{}} \;'.format(path)

    cmd = ["ssh", machine["addr"]]
    if "port" in machine:
        cmd.extend(["-p", machine["port"]])
    cmd.append(find_cmd)

    sp = subprocess.Popen(cmd, stdout=subprocess.PIPE)
    output, _ = sp.communicate()
    output = output.decode("utf-8")
    raw_crashes = output.split("\n")
    crashes = {}
    for raw in raw_crashes:
        toks = raw.split(maxsplit=1)
        if len(toks) < 2:
            continue
        path, desc = toks[0].rsplit("/")[-1], toks[1]
        crashes[path] = desc
    return crashes


def filterout(crash):
    blacklist = ["SYZFAIL", "lost connection", "no output", "suppressed"]
    if len(crash) == 0:
        return True
    for b in blacklist:
        if crash.startswith(b):
            return True
    return False


def print_crashes(name, crashes):
    print(name)
    list_crashes = [(k, v) for k, v in crashes.items()]
    list_crashes.sort(key=lambda x: x[1])
    print(
        *["  " + k + "    " + v for k, v in list_crashes if not filterout(v)], sep="\n"
    )


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--machine", action="store", default="machines.json")
    parser.add_argument("--all", action="store_true", default=False)
    args = parser.parse_args()

    with open(args.machine) as f:
        machines = json.load(f)

    total = {}
    for machine in machines:
        crashes = grab(machine)
        if args.all:
            print_crashes(machine["name"], crashes)
        total = total | crashes

    print_crashes("total", total)


if __name__ == "__main__":
    main()
