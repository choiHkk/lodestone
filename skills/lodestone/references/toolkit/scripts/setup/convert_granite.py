import json
import shutil
from importlib.metadata import version

import mlx.core as mx
from mlx_raclate.utils.utils import load, quantize_model

from convert_common import parse_args, validate, write_manifest


def main():
    args = parse_args(converter_revision=True)
    validate(args, "modernbert")
    model, _ = load(
        str(args.source),
        pipeline="embeddings",
        tokenizer_config={"fix_mistral_regex": True},
    )
    weights, config = quantize_model(
        model,
        model.config.__dict__.copy(),
        q_group_size=args.group_size,
        q_bits=args.bits,
    )
    for key in ("quantization", "quantization_config"):
        config.setdefault(key, {})["mode"] = "affine"

    args.output.mkdir(parents=True)
    try:
        mx.save_safetensors(str(args.output / "model.safetensors"), weights)
        for path in args.source.iterdir():
            if path.name in {"model.safetensors", "config.json"}:
                continue
            target = args.output / path.name
            if path.is_dir():
                shutil.copytree(path, target)
            elif path.is_file():
                shutil.copy2(path, target)
        (args.output / "config.json").write_text(
            json.dumps(config, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )

        write_manifest(
            args,
            {
                "mlx_raclate_revision": args.converter_revision,
                "mlx_version": version("mlx"),
            },
        )
    except BaseException:
        shutil.rmtree(args.output, ignore_errors=True)
        raise


if __name__ == "__main__":
    main()
