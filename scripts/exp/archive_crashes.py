#!python3

import argparse
import os, sys, glob
import datetime
import shutil

def archive_single_file(src, archive, copy=False):
    if not os.path.exists(src):
        print('[-] {} does not exist'.format(src), file=sys.stderr)
        return
    basename = os.path.basename(src)
    dst = os.path.join(archive, basename)
    if copy:
        print("[*] copying {} to {}".format(src, dst))
        shutil.copy(src, dst)
    else:
        print("[*] moving {} to {}".format(src, dst))
        os.rename(src, dst)

def archive_workdir_contents(workdir, archive, pattern):
    srcs = glob.glob(os.path.join(workdir, pattern))
    if len(srcs) == 0:
        print('[-] {} does not exist'.format(pattern), file=sys.stderr)
        return
    for src in srcs:
        archive_single_file(src, archive)

def archive_crashes(workdir, kernel, archive):
    if os.path.isdir(archive):
        print("[-] directory already exists: {}".format(archive), file=sys.stderr)
        return

    crashes_dir = os.path.join(workdir, 'crashes')
    if not os.path.isdir(crashes_dir) or len(os.listdir(crashes_dir)) == 0:
        print("[-] crash not found: {}".format(workdir), file=sys.stderr)
        return

    vmlinux = os.path.join(kernel, 'vmlinux')
    if not os.path.isfile(vmlinux):
        print("[-] vmlinux not found: {}".format(vmlinux), file=sys.stderr)

    bzImage = os.path.join(kernel, 'arch', 'x86', 'boot', 'bzImage')
    if not os.path.isfile(bzImage):
        print("[-] bzImage not found: {}".format(bzImage), file=sys.stderr)

    print('[*] archiving to {}'.format(archive))
    os.makedirs(archive)
    # preserve the workdir hierarchy
    archive_workdir_contents(workdir, archive, 'crashes')
    archive_workdir_contents(workdir, archive, 'bench*.txt')
    archive_workdir_contents(workdir, archive, 'log')
    # copy kernel binaries
    archive_single_file(vmlinux, archive, copy=True)
    archive_single_file(bzImage, archive, copy=True)

def main():
    exp_dir_x86_64 = os.path.join(os.environ['EXP_DIR'], 'x86_64')
    now = datetime.datetime.now().strftime('%y%m%d-%H%M%S')

    default_workdir = os.path.join(exp_dir_x86_64, 'workdir')
    default_kernel = os.environ['KERNEL_X86_64']
    default_archive = os.path.join(exp_dir_x86_64, 'archive', now)

    parser = argparse.ArgumentParser(description='Archive found crashes.')
    parser.add_argument('--workdir', action='store', help='the path to the workdir directory', default=default_workdir)
    parser.add_argument('--kernel', action='store', help='the path to the kernel directory', default=default_kernel)
    parser.add_argument('--archive', action='store', help='the path to the archive directory', default=default_archive)

    args = parser.parse_args()

    archive_crashes(args.workdir, args.kernel, args.archive)

if __name__ == '__main__':
    main()
