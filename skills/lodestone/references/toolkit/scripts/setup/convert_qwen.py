import shutil
from importlib.metadata import version

from mlx_lm.convert import convert

from convert_common import parse_args, validate, write_manifest


def main():
    args = parse_args()
    validate(args, "qwen3")
    try:
        convert(
            hf_path=str(args.source),
            mlx_path=str(args.output),
            quantize=True,
            q_group_size=args.group_size,
            q_bits=args.bits,
            q_mode="affine",
        )
        write_manifest(args, {"mlx_lm_version": version("mlx-lm")})
    except BaseException:
        # A partial output directory would block every later conversion.
        shutil.rmtree(args.output, ignore_errors=True)
        raise


if __name__ == "__main__":
    main()
