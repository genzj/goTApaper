#!python3
import os.path
import sys

from PIL import Image


def main():
    if len(sys.argv) < 2:
        print("No path to original / hi-res icon provided")
        return 1

    for png_file in sys.argv[1:]:
        img = Image.open(png_file)
        if not (os.path.isfile(png_file)):
            print("There is no such file: {}\n".format(sys.argv[1]))
            return 1

        fname, _ = os.path.splitext(png_file)

        img.save(f'{fname}.ico')


if __name__ == '__main__':
    sys.exit(main())
