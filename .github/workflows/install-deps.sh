#! /bin/sh
set -e

if [ "$(uname)" = Darwin ]; then
    brew install go
fi

if [ "$(uname)" = Linux ]; then
    :
fi

if [ "$(uname)" = FreeBSD ]; then
    pkg install -y go
fi

if [ "$(uname)" = NetBSD ]; then
    pkgin -y install go

    for bin in /usr/pkg/bin/go1*; do
        src=$bin
    done
    ln -s "$src" /usr/pkg/bin/go
fi

if [ "$(uname)" = OpenBSD ]; then
    pkg_add -I go
fi
