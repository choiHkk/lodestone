"""Copy every labeled pair's planted source into a repository copy."""

import argparse
import json
import pathlib
import shutil


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--pairs", required=True)
    ap.add_argument("--repo", required=True)
    ap.add_argument("--force", action="store_true", help="overwrite files that already exist")
    args = ap.parse_args()

    repo = pathlib.Path(args.repo).resolve()
    if not repo.is_dir():
        raise SystemExit(f"plant.py: {repo} is not a directory")
    specs = sorted(pathlib.Path(args.pairs).glob("*/pair.json"))
    if not specs:
        raise SystemExit(f"plant.py: no pair.json under {args.pairs}")
    for spec in specs:
        pair = json.loads(spec.read_text(encoding="utf-8"))
        target = (repo / pair["planted"]["into"]).resolve()
        if repo not in target.parents:
            raise SystemExit(f"plant.py: {pair['planted']['into']} escapes {repo}")
        if target.exists() and not args.force:
            raise SystemExit(f"plant.py: {target} already exists; pass --force to overwrite")
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(spec.parent / "planted.go", target)
        print(f"  planted {pair['planted']['func']:<24} into {pair['planted']['into']}")


if __name__ == "__main__":
    main()
