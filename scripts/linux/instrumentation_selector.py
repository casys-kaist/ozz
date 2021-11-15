#!python3

from bs4 import BeautifulSoup
from enum import Enum
import os, re

class Entry:
    class Type(Enum):
        Dir = 0
        File = 1

    Threshold = 40

    def __init__(self, tag):
        if tag == None:
            self.typ = Entry.Type.Dir
            self.path = "path"
            self.level = 1
            self.instrument = False
            self.child = {}
        else:
            self.tag = tag
            self.typ = Entry.Type.Dir if tag.name == 'span' else Entry.Type.File
            if self.typ == Entry.Type.Dir:
                self.child = {}
            self.path = tag['id']
            self.level = len(self.path.split('/'))
            percentage_string = re.compile('[0-9]+%').search(tag.text)[0]
            self.percentage = int(percentage_string[:-1])
            self.instrument = self.percentage > Entry.Threshold
        # Not yet populated
        self.export = False

    def path_level(self, n):
        idx = [i for i, c in enumerate(self.path) if c == '/'][n-1]
        return self.path[:idx]

    def add_node(self, node):
        if node.path.find('arch') != -1 or node.path.find('include') != -1 or node.path.endswith('.h'):
            return
        dirname = os.path.dirname(node.path)
        if dirname == self.path:
            self.child[node.path] = node
        else:
            child_path = node.path_level(self.level+1)
            # print(child_path, self.path, self.level)
            if not child_path in self.child:
                e = Entry(None)
                e.level = len(child_path.split('/'))
                e.path = child_path
                e.instrument = False
                e.percentage = False
                self.child[child_path] = e
            child = self.child[child_path]
            child.add_node(node)

    def populate_export(self):
        if self.typ == Entry.Type.Dir and len(self.child) != 0:
            cnt = sum([1 for path in self.child if self.child[path].populate_export()])
            self.export = (cnt / len(self.child)) * 100 > Entry.Threshold
        return self.instrument

    def print_instrument(self):
        if self.typ == Entry.Type.File:
            if self.instrument:
                print(self)
        else:
            if self.instrument or self.export:
                print(self)
            if not self.export:
                for child_path in self.child:
                    self.child[child_path].print_instrument()

    def __str__(self):
        if self.typ == Entry.Type.Dir:
            prefix = 'D: '
            if self.export:
                prefix = 'E' + prefix
        elif self.typ == Entry.Type.File:
            prefix = 'F: '
        return prefix + self.path[len("path/"):]

def collect_tags(soup, typ):
    tags = soup.find_all(typ, id=re.compile('^path/'))
    r = re.compile('[0-9]+%')
    t = [Entry(tag) for tag in tags if r.search(tag.text) is not None]
    return t

def collect_directory_tags(soup):
    return collect_tags(soup, 'span')

def collect_file_tags(soup):
    return collect_tags(soup, 'a')

def parse_soup(soup):
    directories = collect_directory_tags(soup)
    files = collect_file_tags(soup)
    directories.sort(key = lambda e: len(e.path))
    files.sort(key = lambda e: len(e.path))
    root = Entry(None)
    for directory in directories:
        root.add_node(directory)
    for file in files:
        root.add_node(file)
    root.populate_export()
    root.print_instrument()

def main(args):
    if args.source == 'web':
        import requests
        url = 'https://storage.googleapis.com/syzkaller/cover/{}.html'.format(args.name)
        print('Receiving a html file from {}...'.format(url))
        response = requests.get(url)
        if response.status_code == 200:
            html = response.text
            soup = BeautifulSoup(html, 'html.parser')
        else:
            print('Error code {}'.format(response.status_code), file=sys.stderr)
        print('Receiving a html file from {}... done.'.format(url))
    elif args.source == 'file':
        fn = args.filename
        print('Reading a html file from {}...'.format(fn))
        with open(fn) as f:
            soup = BeautifulSoup(f, 'html.parser')
        print('Reading a html file from {}... done.'.format(fn))
    else:
        print('wrong source', file=sys.stderr)

    parse_soup(soup)

if __name__ == '__main__':
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('--name',  action='store', default='ci-upstream-linux-next-kasan-gce-root', help='the name of a syzkaller instance from which the code coverage is collected')
    parser.add_argument('--source', action='store', default='web', help='the source of the code coverage (web, file)')
    parser.add_argument('--filename', action='store', default='', help='the file name containing html contents if the source is "file"')

    args = parser.parse_args()
    main(args)
