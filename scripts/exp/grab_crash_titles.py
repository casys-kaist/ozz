#!python3
"""Temporary script to grap all titles from all machines
"""
import argparse
import os
import subprocess

def grab(addr, port, workdir):
    path = os.path.join(workdir, "exp/x86_64/workdir/crashes")
    find_cmd = "find {} -name description -exec cat {{}} \;".format(path)
    cmd = ['ssh', addr, '-p', port, find_cmd]
    sp = subprocess.Popen(cmd, stdout=subprocess.PIPE)
    output, _ = sp.communicate()
    output = output.decode("utf-8")
    crashes = output.split('\n')
    return crashes

def filterout(crash):
    if crash.startswith('SYZFAIL'):
        return True
    if crash.startswith('lost connection'):
        return True
    if crash.startswith('no output'):
        return True
    if crash.startswith('WARNING: The mand mount option'):
        return True
    if crash.startswith('WARNING in sbitmap_get'):
        return True
    return False

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--machine', action='store', default='machines.txt')
    args = parser.parse_args()
    with open(args.machine) as f:
        machines = f.readlines()
    for machine in machines:
        name, addr, port, workdir = machine.split()
        print(name)
        crashes = grab(addr, port, workdir)
        for crash in crashes:
            if not filterout(crash):
                print(crash)

if __name__ == "__main__":
    main()
