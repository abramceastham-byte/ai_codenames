#!/usr/bin/env python3
"""Convert GloVe/ConceptNet text vector files to word2vec binary format.

Word2Vec binary format:
  Line 1: "<vocab_size> <dims>\n"
  Per word: "<word> " + little-endian float32 array + "\n"
"""

import struct
import sys
import gzip
import os


def convert(input_path: str, output_path: str, has_header: bool = False):
    open_fn = gzip.open if input_path.endswith(".gz") else open

    print(f"Counting lines in {input_path}...")
    count = 0
    dims = None
    with open_fn(input_path, "rt", encoding="utf-8") as f:
        for i, line in enumerate(f):
            if has_header and i == 0:
                continue
            parts = line.split()
            if dims is None:
                dims = len(parts) - 1
            count += 1

    print(f"  {count} words, {dims} dims")
    print(f"Writing binary to {output_path}...")

    with open_fn(input_path, "rt", encoding="utf-8") as fin, open(output_path, "wb") as fout:
        fout.write(f"{count} {dims}\n".encode("utf-8"))
        for i, line in enumerate(fin):
            if has_header and i == 0:
                continue
            parts = line.rstrip().split()
            word = parts[0]
            floats = [float(x) for x in parts[1:]]
            fout.write(word.encode("utf-8") + b" ")
            fout.write(struct.pack(f"<{len(floats)}f", *floats))
            fout.write(b"\n")
            if (i + 1) % 100000 == 0:
                print(f"  {i + 1} / {count}...")

    print("Done.")


if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Usage: convert_to_w2v_binary.py <input.[txt|txt.gz]> <output.bin> [--has-header]")
        sys.exit(1)
    has_header = "--has-header" in sys.argv
    convert(sys.argv[1], sys.argv[2], has_header=has_header)
