#!/usr/bin/python
# coding: utf-8

# credit: https://retifrav.github.io/blog/2018/10/09/macos-convert-png-to-icns/
# Given proper (512x512x300 dpi/1024x1024x300 dpi) .png image, generate
# iconset folder and .icns icon.
# Ported Python 3 specific code to Python 2.7.10 for Apple Python support
# VikingOSX, 2019-04-04, Apple Support Communities, No Warranty at all.

import subprocess
import os
import sys


class IconParameters():
    width = 0
    scale = 1

    def __init__(self, width, scale, ext):
        self.width = width
        self.scale = scale
        self.ext = ext

    def getIconName(self):
        if self.scale != 1:
            return "icon_{}x{}{}".format(self.width, self.width, self.ext)
        else:
            return "icon_{}x{}@2x{}".format(self.width // 2, self.width // 2,
                                            self.ext)


def main():
    if len(sys.argv) < 2:
        print("No path to original / hi-res icon provided")
        raise SystemExit

    if len(sys.argv) > 2:
        print("Too many arguments")
        raise SystemExit

    originalPicture = sys.argv[1]
    if not (os.path.isfile(originalPicture)):
        print("There is no such file: {}\n".format(sys.argv[1]))
        raise SystemExit

    fname, ext = os.path.splitext(originalPicture)
    destDir = os.path.dirname(originalPicture)
    iconsetDir = os.path.join(destDir, "{}.iconset".format(fname))
    if not (os.path.exists(iconsetDir)):
        os.mkdir(iconsetDir, 0o0755)

    ListOfIconParameters = [
        IconParameters(16, 1, ext),
        IconParameters(16, 2, ext),
        IconParameters(32, 1, ext),
        IconParameters(32, 2, ext),
        IconParameters(64, 1, ext),
        IconParameters(64, 2, ext),
        IconParameters(128, 1, ext),
        IconParameters(128, 2, ext),
        IconParameters(256, 1, ext),
        IconParameters(256, 2, ext),
        IconParameters(512, 1, ext),
        IconParameters(512, 2, ext),
        IconParameters(1024, 1, ext),
        IconParameters(1024, 2, ext)
    ]

    # generate iconset
    for ip in ListOfIconParameters:
        subprocess.call(["sips", "-z", str(ip.width), str(ip.width),
                         originalPicture, "--out",
                         os.path.join(iconsetDir, ip.getIconName())])
        # print("Generated: {}\n".format(ip.getIconName()))

    # convert iconset to icns file
    subprocess.call(["iconutil", "-c", "icns", iconsetDir, "-o",
                     os.path.join(destDir, "{}.icns".format(fname))])


if __name__ == '__main__':
    sys.exit(main())
