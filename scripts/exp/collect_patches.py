#!/usr/bin/env python

"""Collecting patches that fixed a bug caused by missing (or incorrect) memory barrier(s)

CAUTION: This script does not guarantee to find all patches, nor
exclude patches actually not fixing such a bug. This just tries its
best heuristic.

"""

import os
import re
import sys
import time

from git import Repo


class Analyzer:
    def is_merge_commit(commit):
        return len(commit.parents) > 1

    def is_initial_commit(commit):
        return len(commit.parents) == 0

    def get_change(commit):
        message = commit.message
        parent = commit.parents[0]
        diffs = parent.diff(commit, create_patch=True)
        return message, diffs

    def is_interesting(self, message, diffs):
        return False, []

    def is_comment(line):
        return (
            re.match("\+\s*\*", line) != None
            or re.match("\+\s*//", line) != None
            or re.match("\+\s*/\*", line)
        )


# TODO: make this more precise
class MembarrierAnalyzer(Analyzer):
    def __message_containing_hint(message):
        return message.find("memory barrier") != -1 or (
            message.find("reordering") != -1 and message.find("fixing") != -1
        )

    def __message_containing_calltrace(message):
        return message.find("Call trace") != -1

    def __is_membarrier_insertion(line):
        if not line.startswith("+"):
            return False
        if Analyzer.is_comment(line):
            return False
        return (
            line.find("smp_mb") != -1
            or line.find("smp_store_release") != -1
            or line.find("smp_wmb") != -1
        )

    def __diffs_membarrier_dominant(commit, diffs):
        total_insertions = commit.stats.total["insertions"]
        membarrier_insertions = 0
        comment_insertions = 0
        empty_insertions = 0
        threshold = 0.3
        try:
            for diff in diffs:
                lines = diff.diff.split(b"\n")
                membarrier_insertions += sum(
                    [
                        1
                        for line in lines
                        if MembarrierAnalyzer.__is_membarrier_insertion(line.decode())
                    ]
                )
                comment_insertions += sum(
                    [1 for line in lines if Analyzer.is_comment(line.decode())]
                )
                empty_insertions += sum([1 for line in lines if line == "+"])
        except:
            return False
        total_insertions -= comment_insertions
        total_insertions -= empty_insertions
        return membarrier_insertions > total_insertions * threshold

    def is_interesting(self, commit):
        # Skip merge commits as they are unlikely interested
        if Analyzer.is_merge_commit(commit) or Analyzer.is_initial_commit(commit):
            return False, (False, False)

        message, diffs = Analyzer.get_change(commit)

        # At this point, we want to focus on simple and definite bug patches
        if not MembarrierAnalyzer.__diffs_membarrier_dominant(commit, diffs):
            return False, (False, False)

        message_hint = MembarrierAnalyzer.__message_containing_hint(message)
        calltrace = MembarrierAnalyzer.__message_containing_calltrace(message)

        return True, (calltrace, message_hint)


class Inspector:
    def __init__(self, working_tree_dir, analyzer):
        self.working_tree_dir = working_tree_dir
        assert len(self.working_tree_dir) != 0
        assert os.path.isdir(self.working_tree_dir)

        self.repo = Repo(self.working_tree_dir)
        assert not self.repo.bare

        self.result = []

        assert isinstance(analyzer, Analyzer)
        self.analyzer = analyzer

    def __inspect_commit(self, commit):
        ok, info = self.analyzer.is_interesting(commit)
        if ok:
            print("[+] {}".format(commit))
            self.result.append([commit, info])

    def inspect(self, branch="master", max_count=-1, timeout=10 * 60 * 60):
        start = time.time()
        for i, commit in enumerate(self.repo.iter_commits(branch, max_count=max_count)):
            if i % 1000 == 0:
                print("[*] Inspecting {}th commit".format(i))
            if time.time() - start > timeout:
                break
            self.__inspect_commit(commit)
        self.result.sort(reverse=True, key=lambda x: x[1])


def main():
    if len(sys.argv) >= 2:
        linux_working_dir = sys.argv[1]
    else:
        kernels_dir = os.environ["KERNELS_DIR"]
        linux_working_dir = os.path.join(kernels_dir, "linux")

    if len(sys.argv) >= 3:
        branch = sys.argv[2]
    else:
        branch = "master"

    inspector = Inspector(linux_working_dir, MembarrierAnalyzer())
    print("[*] Starting inspection")
    inspector.inspect(branch=branch)

    print("Result:")
    for result in inspector.result:
        print(result)


if __name__ == "__main__":
    main()
