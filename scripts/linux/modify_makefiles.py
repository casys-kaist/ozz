#!python3

"""
Script to modify Makefiles in Linux to instrument KSSB
"""

import os, sys

Makefile = "Makefile"


def reset_makefile(makefile):
    def contain(string, substring):
        return string.find(substring) != -1

    KSSB_INSTRUMENT = "KSSB_INSTRUMENT"
    KSSB_FORCE = "# KSSB force"
    new = []
    with open(makefile, "r+") as f:
        modified = False
        for line in f.readlines():
            if not contain(line, KSSB_INSTRUMENT) or contain(line, KSSB_FORCE):
                new.append(line)
            else:
                modified = True
        if modified:
            print("[*] Resetting Makefile {}".format(makefile))
            f.seek(0)
            f.write("".join(new))
            f.truncate()


def reset_makefiles(kernel):
    for root, dirs, files in os.walk(kernel):
        if Makefile in files:
            reset_makefile(os.path.join(root, Makefile))


def do_modify_makefile(kernel, directive, path, makefile, misc=None):
    export = directive.find("E") != -1
    directory = directive.find("D") != -1
    fil = not directory
    if not os.path.isfile(makefile):
        print(
            "\033[31m"
            + "[-]"
            + "\033[0m"
            + " Makefile does not exist: {}".format(makefile),
            file=sys.stderr,
        )
        return
    if fil and not os.path.isfile(os.path.join(kernel, path)):
        print(
            "\033[31m"
            + "[-]"
            + "\033[0m"
            + " Basetarget does not exist: {}".format(path),
            file=sys.stderr,
        )
        return
    with open(makefile, "r+") as f:
        comments, lines = [], f.readlines()
        for i, line in enumerate(lines):
            if line.startswith("#"):
                continue
            comments, lines = lines[:i], lines[i:]
            break
        new = []
        if directory:
            new.append("KSSB_INSTRUMENT := y # auto generated\n")
        if fil:
            misc = misc[:-1] + "o"
            new.append("KSSB_INSTRUMENT_{} := y # auto generated\n".format(misc))
        if export:
            new.append("export KSSB_INSTRUMENT # auto generated\n")
        if len(new) == 0:
            return
        contents = comments + new + lines
        print("[*] Modifing Makefile {}".format(makefile))
        f.seek(0)
        f.write("".join(contents))
        f.truncate()


def modify_makefile_file(kernel, directive, path):
    idx = path.rfind("/")
    basetarget = path[idx + 1 :]
    makefile = os.path.join(kernel, path[:idx], Makefile)
    do_modify_makefile(kernel, directive, path, makefile, basetarget)


def modify_makefile_directory(kernel, directive, path):
    makefile = os.path.join(kernel, path, Makefile)
    do_modify_makefile(kernel, directive, path, makefile)


def modify_makefile(kernel, recipe):
    recipe = recipe.strip()
    directive, path = recipe.split(": ")
    if directive == "F":
        modify_makefile_file(kernel, directive, path)
    else:
        modify_makefile_directory(kernel, directive, path)


def main(args):
    reset_makefiles(args.kernel)
    with open(args.recipe) as f:
        for recipe in f.readlines():
            modify_makefile(args.kernel, recipe)


if __name__ == "__main__":
    kernel_path = os.path.join(os.environ["KERNELS_DIR"], "linux")
    recipe_path = os.path.join(os.environ["TMP_DIR"], "instrument")

    import argparse

    parser = argparse.ArgumentParser()
    parser.add_argument("--recipe", action="store", default=recipe_path)
    parser.add_argument("--kernel", action="store", default=kernel_path)

    args = parser.parse_args()
    main(args)
