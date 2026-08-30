"""Directed rank of a known pair under one embedding setting.

Reads units as JSONL from dumpfuncs, embeds every unit through the MLX runtime,
and reports where the known partner lands among the query unit's own candidates.
The spread matters as much as the rank: a setting that orders correctly but
compresses every score into a narrow band leaves nowhere to put a threshold.
"""

import argparse
import json
import statistics
import subprocess
import tempfile


class RuntimeRefused(Exception):
    """The runtime would not serve this model, usually a model_type it cannot decode."""


def embed(texts, runtime, model, batch, max_tokens):
    with tempfile.TemporaryFile(mode="w+", encoding="utf-8") as stderr:
        proc = subprocess.Popen(
            [runtime, "--model", model, "--max-batch", str(batch), "--max-tokens", str(max_tokens)],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=stderr, text=True, bufsize=1)
        out = []
        for start in range(0, len(texts), batch):
            proc.stdin.write(json.dumps({"id": start, "texts": texts[start:start + batch]}) + "\n")
            proc.stdin.flush()
            line = proc.stdout.readline()
            if not line:
                proc.stdin.close()
                proc.wait()
                stderr.seek(0)
                raise RuntimeRefused((stderr.read().strip() or "no output").splitlines()[0])
            response = json.loads(line)
            if response.get("error"):
                proc.stdin.close()
                proc.wait()
                raise RuntimeRefused(response["error"])
            if response.get("id") != start:
                proc.stdin.close()
                proc.wait()
                raise RuntimeRefused(f"response id {response.get('id')} for request {start}")
            out.extend(response["embeddings"])
            if len(response["embeddings"]) != len(texts[start:start + batch]):
                proc.stdin.close()
                proc.wait()
                raise RuntimeRefused("embedding count does not match the request")
        proc.stdin.close()
        if proc.wait() != 0:
            stderr.seek(0)
            raise RuntimeRefused(f"runtime exited with {proc.returncode}: {stderr.read().strip() or 'no output'}")
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--units", required=True, help="JSONL from harness/cmd/dumpfuncs")
    ap.add_argument("--runtime", required=True)
    ap.add_argument("--model", required=True)
    ap.add_argument("--query", required=True, help="unit name to search from")
    ap.add_argument("--partner", required=True, help="unit name that is the known answer")
    ap.add_argument("--instruction", default="", help="query prefix; empty means symmetric")
    ap.add_argument("--label", default="")
    ap.add_argument("--batch", type=int, default=8)
    ap.add_argument("--max-tokens", type=int, default=768)
    args = ap.parse_args()

    with open(args.units, encoding="utf-8") as handle:
        units = [json.loads(line) for line in handle]
    if len(units) < 2:
        raise SystemExit("rank.py: need at least two units")
    sources = [u["source"] for u in units]
    index = {}
    duplicated = set()
    for i, u in enumerate(units):
        if u["name"] in index:
            duplicated.add(u["name"])
        index[u["name"]] = i
    for unit in (args.query, args.partner):
        if unit not in index:
            raise SystemExit(f"rank.py: unit {unit!r} not in {args.units}")
        if unit in duplicated:
            raise SystemExit(f"rank.py: unit name {unit!r} is ambiguous in {args.units}; disambiguate the dump")
    if args.query == args.partner:
        raise SystemExit("rank.py: --query and --partner must differ")
    name = args.model.rstrip("/").rsplit("/", 1)[-1]
    try:
        docs = embed(sources, args.runtime, args.model, args.batch, args.max_tokens)
    except RuntimeRefused as refusal:
        print(json.dumps({"model": name, "refused": str(refusal)}))
        return

    q = index[args.query]

    dot = lambda a, b: sum(x * y for x, y in zip(a, b))
    if args.instruction:
        try:
            queries = embed([args.instruction + s for s in sources], args.runtime, args.model, args.batch, args.max_tokens)
        except RuntimeRefused as refusal:
            print(json.dumps({"model": name, "refused": str(refusal)}))
            return
        score = lambda i: (dot(queries[q], docs[i]) + dot(queries[i], docs[q])) / 2
    else:
        score = lambda i: dot(docs[q], docs[i])

    ranked = sorted(((score(i), units[i]["name"]) for i in range(len(units)) if i != q), reverse=True)
    rank = next(i for i, (_, unit) in enumerate(ranked, 1) if unit == args.partner)
    values = [s for s, _ in ranked]

    print(json.dumps({
        "label": args.label or ("symmetric" if not args.instruction else "instructed"),
        "model": name,
        "rank": rank,
        "of": len(ranked),
        "score": round(ranked[rank - 1][0], 4),
        "max": round(max(values), 4),
        "min": round(min(values), 4),
        "median": round(statistics.median(values), 4),
        "spread": round(max(values) - min(values), 4),
        "top3": [[round(s, 4), n] for s, n in ranked[:3]],
    }))


if __name__ == "__main__":
    main()
