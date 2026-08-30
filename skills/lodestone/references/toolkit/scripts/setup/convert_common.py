import argparse
import json
from pathlib import Path


def parse_args(converter_revision=False):
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--revision", required=True)
    if converter_revision:
        parser.add_argument("--converter-revision", required=True)
    parser.add_argument("--group-size", default=64, type=int)
    parser.add_argument("--bits", default=8, type=int)
    return parser.parse_args()


def validate(args, model_type):
    if args.output.exists():
        raise FileExistsError(f"output already exists: {args.output}")
    source_config = json.loads((args.source / "config.json").read_text(encoding="utf-8"))
    if source_config.get("model_type") != model_type:
        raise ValueError(f"source model_type must be {model_type}")


def write_manifest(args, converter):
    manifest = {
        "bits": args.bits,
        "format": "mlx",
        "group_size": args.group_size,
        "mode": "affine",
        "repository": args.repository,
        "revision": args.revision,
    }
    manifest.update(converter)
    (args.output / "conversion.json").write_text(
        json.dumps(manifest, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
