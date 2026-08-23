#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import argparse
import os
import sys
import zipfile

fmt = '\x1b[{:d};{:d}m{:s}\x1b[0m'  # terminal color template


def decode_name(name):
    try:
        return name.encode('cp437').decode('gb2312')
    except (UnicodeEncodeError, UnicodeDecodeError):
        return name


def single_directory_prefix(members):
    names = [decode_name(member.filename).replace('\\', '/') for member in members]
    names = [name.lstrip('/') for name in names if name.rstrip('/')]
    top_level_names = {name.split('/', 1)[0] for name in names}
    if len(top_level_names) != 1:
        return None

    top_level_name = next(iter(top_level_names))
    is_directory = any('/' in name.rstrip('/') for name in names)
    is_directory = is_directory or any(
        name.rstrip('/') == top_level_name and member.is_dir()
        for name, member in zip(names, members)
    )
    return top_level_name if is_directory else None


def safe_destination(base_directory, relative_name):
    base_directory = os.path.realpath(os.path.abspath(base_directory))
    destination = os.path.abspath(os.path.join(base_directory, relative_name))
    resolved_destination = os.path.realpath(destination)
    if os.path.commonpath([base_directory, resolved_destination]) != base_directory:
        raise ValueError("unsafe path")
    return destination


def unzip(path, secret=None, output_dir=None):
    with zipfile.ZipFile(path, "r") as archive:
        if secret:
            if isinstance(secret, str):
                secret = secret.encode()
            archive.setpassword(secret)

        members = archive.infolist()
        prefix = single_directory_prefix(members) if output_dir else None
        base_directory = output_dir or os.curdir
        os.makedirs(base_directory, exist_ok=True)

        for member in members:
            decoded_name = decode_name(member.filename).replace('\\', '/')
            relative_name = decoded_name.lstrip('/')
            if prefix:
                if relative_name.rstrip('/') == prefix:
                    continue
                relative_name = relative_name[len(prefix) + 1:]
            if not relative_name:
                continue

            try:
                destination = safe_destination(base_directory, relative_name)
                if member.is_dir():
                    os.makedirs(destination, exist_ok=True)
                    continue

                os.makedirs(os.path.dirname(destination), exist_ok=True)
                if not os.path.exists(destination):
                    with open(destination, "wb") as output_file:
                        output_file.write(archive.read(member))
            except Exception as err:
                print("!!! write {}:".format(relative_name), err)

def main(argv):
    p = argparse.ArgumentParser(description='解决unzip乱码')
    p.add_argument('archives', type=str, nargs='*', help='ZIP 文件')
    p.add_argument('-s', '--secret', action='store', default=None, help='密码')
    p.add_argument('--output-dir', help='将解压结果放入指定目录')

    args = p.parse_args(argv[1:])

    for path in args.archives:
        if path.endswith('.zip'):
            if os.path.exists(path):
                print(fmt.format(1, 97, "  ++ unzip:"), path)
                unzip(path, secret=args.secret, output_dir=args.output_dir)
            else:
                print(fmt.format(1, 91, "  !! file doesn't exist."), path)
        else:
            print(fmt.format(1, 91, "  !! file isn't a zip file."), path)

if __name__ == '__main__':
    argv = sys.argv
    main(argv)
