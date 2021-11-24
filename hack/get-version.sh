#!/bin/sh

git describe --tags | cut -d\- -f1

