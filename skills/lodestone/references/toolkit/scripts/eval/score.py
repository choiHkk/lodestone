"""Say where each labeled pair landed in one report.

Settings were chosen against the tune pairs, so a tune row shows only that the
choice took effect. Holdout rows are the evidence that it generalises.
"""

import argparse
import json
import pathlib


def bare(name):
    return name.rsplit(".", 1)[-1]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--pairs", required=True)
    ap.add_argument("--report", required=True)
    args = ap.parse_args()

    with open(args.report, encoding="utf-8") as handle:
        rows = json.load(handle).get("candidates") or []
    by_separation = sorted(range(len(rows)), key=lambda i: -rows[i].get("separation", 0))

    found = {"tune": [0, 0], "holdout": [0, 0]}
    print(f"  {'pair':<26} {'split':<8} {'fused':>8} {'sep-rank':>12}")
    for spec in sorted(pathlib.Path(args.pairs).glob("*/pair.json")):
        pair = json.loads(spec.read_text(encoding="utf-8"))
        want = {bare(pair["donor"]["func"]), bare(pair["planted"]["func"])}
        planted_file = pair["planted"]["into"]
        hit = [i for i, row in enumerate(rows)
               if want == {bare(row["left"]["name"]), bare(row["right"]["name"])}
               and planted_file in (row["left"]["file"], row["right"]["file"])]

        split = pair.get("split")
        if split not in found:
            raise SystemExit(f"score.py: unknown split {split!r} in {spec}")
        found[split][1] += 1
        if hit:
            found[split][0] += 1
            place = f"{hit[0] + 1}/{len(rows)}"
            rank = f"{by_separation.index(hit[0]) + 1}/{len(rows)}"
        else:
            place, rank = "absent", "-"
        print(f"  {spec.parent.name:<26} {split:<8} {place:>8} {rank:>12}")

    print()
    for split in ("tune", "holdout"):
        hit, total = found[split]
        print(f"  {split:<8} {hit}/{total} present")
    print("\n  Only the holdout rows are evidence that a setting generalises.")


if __name__ == "__main__":
    main()
