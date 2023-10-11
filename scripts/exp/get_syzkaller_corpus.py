#!/usr/bin/env python

"""A simple script to extract the Syzkaller's corpus"""

import hashlib
import os

URL = "https://storage.googleapis.com/syzkaller/cover/{}.html"


def get_cover_html_from_file(filename):
    with open(filename) as f:
        contents = f.read()
    return contents


def get_cover_html_from_web(inst):
    import requests

    url = URL.format(inst)
    response = requests.get(url)
    if response.status_code == 200:
        html = response.text
        return html
    else:
        raise Exception(
            "Failed to retrieve the HTML file. Status={}".format(response.status_code)
        )


def get_cover_html(inst, filename):
    """If filename exists, read the file and return it. If not,
    retrieve the html file from the Syzkaller's dashboard"""
    try:
        contents = get_cover_html_from_file(filename)
    except:
        contents = get_cover_html_from_web(inst)
    return contents


def extract_progs_from_html(html):
    import re

    from bs4 import BeautifulSoup

    soup = BeautifulSoup(html, "html.parser")
    progs = []
    for prog_elem in soup.find_all("pre", id=re.compile("^prog_")):
        prog = prog_elem.get_text()
        prog = re.sub(r"^$\n", "", prog, flags=re.MULTILINE)
        progs.append(prog)
    return progs


def hash(data):
    h = hashlib.sha1()
    h.update(data)
    return h.hexdigest()


def save_raw_corpus(progs, outdir):
    raw_corpus_path = os.path.join(outdir, "raw_corpus")
    os.makedirs(raw_corpus_path)
    for prog in progs:
        hsh = hash(prog.encode("utf-8"))
        path = os.path.join(raw_corpus_path, hsh)
        with open(path, "w") as f:
            f.write(prog)
    return raw_corpus_path


def pack_corpus(raw_corpus, outdir):
    import subprocess

    corpus_filename = "corpus.db"
    corpus_path = os.path.join(outdir, corpus_filename)
    curver = 4
    cmd = ["syz-db", "--version={}".format(curver), "pack", raw_corpus, corpus_path]
    subprocess.run(cmd)


def main():
    import argparse

    tmpdir_env_var = "TMP_DIR"
    tmpdir_global = "/tmp"
    tmpdir = (
        os.environ[tmpdir_env_var] if tmpdir_env_var in os.environ else tmpdir_global
    )

    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--instance", action="store", default="ci-upstream-kasan-gce-root"
    )
    parser.add_argument("--file", action="store", default="")
    parser.add_argument("--outdir", action="store", default=tmpdir)
    args = parser.parse_args()

    html = get_cover_html(args.instance, args.file)

    progs = extract_progs_from_html(html)

    raw_corpus_path = save_raw_corpus(progs, args.outdir)

    pack_corpus(raw_corpus_path, args.outdir)


if __name__ == "__main__":
    main()
