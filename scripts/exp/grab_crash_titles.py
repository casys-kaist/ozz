#!/usr/bin/env python

"""script to grap all titles from machines"""

import argparse
import json
import os
import subprocess
import sys

from bs4 import BeautifulSoup


def grab_crashes_from_machine(machine, old_crashes):
    path = machine["workdir"]
    if not old_crashes:
        path = os.path.join(path, "crashes")
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


def filterout(title, fixed, print_starvation):
    blacklist = ["SYZFAIL", "lost connection", "no output", "suppressed"]
    starvation_list = ["rcu detected stall", "unregister_netdevice"]
    if not print_starvation:
        starvation_list.append("hung")
    return (
        len(title) == 0
        or any(title.startswith(b) for b in blacklist)
        or any(f.startswith(title) for f in fixed)
        or any(title.find(s) != -1 for s in starvation_list)
    )


def print_crashes(name, crashes, open_titles, fixed_titles, print_starvation):
    print(name)
    list_crashes = [(k, v) for k, v in crashes.items()]
    list_crashes.sort(key=lambda x: x[1])

    def may_incomplete_fix(v, fixed_titles):
        return (
            "(incomplete fix?)"
            if any(v.strip() == title.strip() for title in fixed_titles)
            else ""
        )

    print(
        *[
            "  "
            + k
            + "    "
            + v
            + " " * (90 - len(v))
            + may_incomplete_fix(v, fixed_titles)
            for k, v in list_crashes
            if not filterout(v, open_titles, print_starvation)
        ],
        sep="\n"
    )


def grab_titles(table):
    title_tags = table.find_all("td", {"class": "title"})
    titles = set()
    for tag in title_tags:
        title = tag.get_text()
        titles.add(title)
    return titles


def retrieve_titles_from_soup(soup, desires):
    tables = []

    def add_table(table):
        if table == None:
            raise Exception("Unknown table")
        else:
            tables.append(table)

    if len(desires) == 0:
        # There is only one table and it is what we want
        table = soup.find("table", {"class": "list_table"})
        add_table(table)
    else:
        for caption_tag in soup.find_all("caption"):
            caption = caption_tag.get_text().strip()
            if any(caption.startswith(d) for d in desires):
                table = caption_tag.find_parent("table", {"class": "list_table"})
                add_table(table)

    titles = set()
    for table in tables:
        titles = titles | grab_titles(table)
    return titles


def crawl_syzkaller_crash_titles(args):
    import requests

    url_open = "https://syzkaller.appspot.com/upstream"
    url_fixed = "https://syzkaller.appspot.com/upstream/fixed"
    urls = []

    check_open, check_fixed = False, False
    if args.unfixed:
        check_open, check_fixed, = (
            False,
            True,
        )
    else:
        check_open, check_fixed, = (
            True,
            True,
        )

    open_titles, fixed_titles = set(), set()

    urls = [
        ("open", ["open", "moderation"], url_open, check_open),
        ("fixed", [], url_fixed, check_fixed),
    ]

    for name, captions, url, check in urls:
        if not check:
            continue
        response = requests.get(url)
        if response.status_code == 200:
            html = response.text
            soup = BeautifulSoup(html, "html.parser")
            if name == "open":
                open_titles = open_titles | retrieve_titles_from_soup(soup, captions)
            else:
                fixed_titles = fixed_titles | retrieve_titles_from_soup(soup, captions)
        else:
            print(
                "Error during crawling {} (code={}). Skipping".format(
                    name, response.status_code
                ),
                file=sys.stderr,
            )

    if args.unfixed:
        open_titles = fixed_titles

    return open_titles, fixed_titles


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--machine", action="store", default="machines.json")
    parser.add_argument("--all", action="store_true")
    parser.add_argument("--unfixed", action="store_true")
    parser.add_argument("--no-starvation", action="store_true")
    parser.add_argument("--old_crashes", action="store_true")
    parser.add_argument("--verbose", action="store_true")
    args = parser.parse_args()

    with open(args.machine) as f:
        machines = json.load(f)

    open_titles, fixed_titles = crawl_syzkaller_crash_titles(args)
    if args.verbose:
        print("open")
        for f in open_titles:
            print("  " + f)
        print("fixed")
        for f in fixed_titles:
            print("  " + f)

    print_starvation = not args.no_starvation

    total = {}
    for machine in machines:
        crashes = grab_crashes_from_machine(machine, args.old_crashes)
        if args.all:
            print_crashes(
                machine["name"], crashes, open_titles, fixed_titles, print_starvation
            )
        total = total | crashes

    print_crashes("total", total, open_titles, fixed_titles, print_starvation)


if __name__ == "__main__":
    main()
