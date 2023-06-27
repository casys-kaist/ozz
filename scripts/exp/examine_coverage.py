#!python3

import sys
import os, argparse

def get_coverage(args, typ):
    path = os.path.join(args.workdir, 'coverage', args.prefix+'.'+typ)
    with open(path) as f:
        coverage = f.readlines()
    return coverage

def strip_prefix(x):
    pat = "linux/"
    idx = x.find(pat)
    return x if idx == -1 else x[idx+len(pat):]

def do_addr2line(coverage, kernel, output, suffix):
    outfile = sys.stdout
    if output != "" and output != None:
        path = output+'.'+suffix
        outfile = open(path, 'w+')
        rawfile = open(path+'-raw', 'w+')
    else:
        print(suffix)

    import subprocess
    covs, hops = [], []
    for raw in coverage:
        raw = raw.strip()
        splited = raw.split()
        hops.append(len(splited))
        covs.extend(map(lambda x: hex(int(x,16)-5 + 0xffffffff00000000), splited))

    res = []
    step = 10000
    for i in range(0, len(covs), step):
        args = ['addr2line', '-e', os.path.join(kernel, 'vmlinux')]
        args.extend(covs[i:i+step])
        done = subprocess.run(args, capture_output=True)
        done_str = done.stdout.decode('utf-8')
        res.extend(done_str.split('\n')[:-1])

    i = 0
    for hop in hops:
        print("    ".join(list(map(lambda x: strip_prefix(x), res[i:i+hop]))), file=outfile)
        print("    ".join(covs[i:i+hop]), file=rawfile)
        i += hop

def do_examine(args):
    code_coverage = get_coverage(args, 'code')
    read_from_coverage = get_coverage(args, 'readfrom')

    if args.addr2line:
        do_addr2line(code_coverage, args.kernel, args.output, 'code')
        do_addr2line(read_from_coverage, args.kernel, args.output, 'readfrom')

def main():
    parser = argparse.ArgumentParser(description='examining coverages')
    parser.add_argument('workdir', action='store', help='workdir')
    parser.add_argument('prefix', action='store', help='prefix of coverage files')
    parser.add_argument('kernel', action='store', help='path to the kernel build')
    parser.add_argument('--addr2line', dest='addr2line', action='store_true', help='translate addressses into lines')
    parser.add_argument('--no-addr2line', dest='addr2line', action='store_false', help='translate addressses into lines')
    parser.add_argument('--output', action='store', help='file name to store thre result')
    parser.set_defaults(addr2line=True)
    args = parser.parse_args()

    do_examine(args)

if __name__ == "__main__":
    main()
