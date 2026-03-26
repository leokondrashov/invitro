from __future__ import annotations

import argparse
import logging as log
from pathlib import Path

import numpy as np
import pandas as pd

from reduce import (
    get_invocation_columns,
    reduce_trace_cache_aware,
    reduce_trace_round_robin,
)


log.basicConfig(format="%(levelname)s:%(message)s", level=log.INFO)


def load_trace(trace_dir: Path) -> tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    if not trace_dir.exists():
        raise RuntimeError(f"Input trace folder does not exist: {trace_dir}")

    inv_df = pd.read_csv(trace_dir / "invocations.csv")
    mem_df = pd.read_csv(trace_dir / "memory.csv")
    run_df = pd.read_csv(trace_dir / "durations.csv")
    return inv_df, mem_df, run_df


def ensure_output_dir(path: Path) -> None:
    path.mkdir(parents=True, exist_ok=True)


def run_reduce(args: argparse.Namespace) -> None:
    trace_dir = Path(args.source_trace)
    output_dir = Path(args.output)
    ensure_output_dir(output_dir)

    inv_df, mem_df, run_df = load_trace(trace_dir)
    invocation_columns = get_invocation_columns(inv_df)
    total_before = int(inv_df[invocation_columns].to_numpy().sum())

    if args.policy == "round-robin":
        reduced_inv_df, reduced_mem_df, reduced_run_df, factor = reduce_trace_round_robin(
            inv_df=inv_df,
            mem_df=mem_df,
            run_df=run_df,
            real_nodes=args.real_nodes,
            max_nodes=args.max_nodes,
            seed=args.seed,
        )
        reduced_inv_df.to_csv(output_dir / "invocations.csv", index=False)
        reduced_mem_df.to_csv(output_dir / "memory.csv", index=False)
        reduced_run_df.to_csv(output_dir / "durations.csv", index=False)

        total_after = int(reduced_inv_df[invocation_columns].to_numpy().sum()) if len(reduced_inv_df) > 0 else 0
        log.info(
            "Reduced trace with round-robin thinning: factor=%.4f (%d/%d), functions %d -> %d, invocations %d -> %d",
            factor,
            args.real_nodes,
            args.max_nodes,
            len(inv_df),
            len(reduced_inv_df),
            total_before,
            total_after,
        )
        return

    simulation_df = pd.read_csv(args.ca_trace_csv) if args.ca_trace_csv is not None else None
    reduced_inv_df, reduced_mem_df, reduced_run_df, report_df, real_node_ids = reduce_trace_cache_aware(
        inv_df=inv_df,
        mem_df=mem_df,
        run_df=run_df,
        real_nodes=args.real_nodes,
        max_nodes=args.max_nodes,
        seed=args.seed,
        simulation_df=simulation_df,
        span_stat=args.ca_span_stat,
    )
    reduced_inv_df.to_csv(output_dir / "invocations.csv", index=False)
    reduced_mem_df.to_csv(output_dir / "memory.csv", index=False)
    reduced_run_df.to_csv(output_dir / "durations.csv", index=False)
    report_df.to_csv(output_dir / "reduction_report.csv", index=False)
    pd.DataFrame({"node_id": real_node_ids}).to_csv(output_dir / "real_nodes.csv", index=False)

    total_after = int(reduced_inv_df[invocation_columns].to_numpy().sum()) if len(reduced_inv_df) > 0 else 0
    log.info(
        "Reduced trace with cache-aware thinning: functions %d -> %d, invocations %d -> %d, mean_factor=%.4f, mean_node_span=%.2f, zero_factor_functions=%d",
        len(inv_df),
        len(reduced_inv_df),
        total_before,
        total_after,
        float(report_df["placement_factor"].mean()),
        float(report_df["node_span"].mean()),
        int((report_df["placement_factor"] == 0).sum()),
    )


def append_round_robin_row(
    rows: list[dict[str, float | int | str]],
    inv_df: pd.DataFrame,
    mem_df: pd.DataFrame,
    run_df: pd.DataFrame,
    total_before: int,
    invocation_columns: list[str],
    real_nodes: int,
    max_nodes: int,
    seed: int,
) -> None:
    reduced_inv_df, _, _, factor = reduce_trace_round_robin(
        inv_df=inv_df,
        mem_df=mem_df,
        run_df=run_df,
        real_nodes=real_nodes,
        max_nodes=max_nodes,
        seed=seed,
    )
    rows.append(
        {
            "policy": "round-robin",
            "seed": seed,
            "functions_before": len(inv_df),
            "functions_after": len(reduced_inv_df),
            "invocations_before": total_before,
            "invocations_after": int(reduced_inv_df[invocation_columns].to_numpy().sum()) if len(reduced_inv_df) else 0,
            "mean_factor": float(factor),
            "mean_node_span": np.nan,
            "zero_factor_functions": np.nan,
            "zero_span_functions": np.nan,
        }
    )


def append_cache_aware_row(
    rows: list[dict[str, float | int | str]],
    inv_df: pd.DataFrame,
    mem_df: pd.DataFrame,
    run_df: pd.DataFrame,
    simulation_df: pd.DataFrame | None,
    total_before: int,
    invocation_columns: list[str],
    real_nodes: int,
    max_nodes: int,
    seed: int,
    span_stat: str,
) -> None:
    reduced_inv_df, _, _, report_df, _ = reduce_trace_cache_aware(
        inv_df=inv_df,
        mem_df=mem_df,
        run_df=run_df,
        real_nodes=real_nodes,
        max_nodes=max_nodes,
        seed=seed,
        simulation_df=simulation_df,
        span_stat=span_stat,
    )
    rows.append(
        {
            "policy": "cache-aware",
            "seed": seed,
            "functions_before": len(inv_df),
            "functions_after": len(reduced_inv_df),
            "invocations_before": total_before,
            "invocations_after": int(reduced_inv_df[invocation_columns].to_numpy().sum()) if len(reduced_inv_df) else 0,
            "mean_factor": float(report_df["placement_factor"].mean()),
            "mean_node_span": float(report_df["node_span"].mean()),
            "zero_factor_functions": int((report_df["placement_factor"] == 0).sum()),
            "zero_span_functions": int((report_df["node_span"] == 0).sum()),
        }
    )


def summarize_results(results: pd.DataFrame) -> pd.DataFrame:
    summary_rows: list[dict[str, float | int | str]] = []
    for policy, grp in results.groupby("policy"):
        summary_rows.append(
            {
                "policy": policy,
                "seed_count": int(len(grp)),
                "functions_after_mean": float(grp["functions_after"].mean()),
                "functions_after_std": float(grp["functions_after"].std(ddof=0)),
                "functions_after_median": float(grp["functions_after"].median()),
                "functions_after_p05": float(grp["functions_after"].quantile(0.05)),
                "functions_after_p95": float(grp["functions_after"].quantile(0.95)),
                "functions_after_min": int(grp["functions_after"].min()),
                "functions_after_max": int(grp["functions_after"].max()),
                "invocations_after_mean": float(grp["invocations_after"].mean()),
                "invocations_after_std": float(grp["invocations_after"].std(ddof=0)),
                "invocations_after_median": float(grp["invocations_after"].median()),
                "invocations_after_p05": float(grp["invocations_after"].quantile(0.05)),
                "invocations_after_p95": float(grp["invocations_after"].quantile(0.95)),
                "invocations_after_min": int(grp["invocations_after"].min()),
                "invocations_after_max": int(grp["invocations_after"].max()),
                "mean_factor_mean": float(grp["mean_factor"].mean(skipna=True)) if grp["mean_factor"].notna().any() else np.nan,
                "mean_factor_std": float(grp["mean_factor"].std(ddof=0, skipna=True)) if grp["mean_factor"].notna().any() else np.nan,
                "mean_node_span_mean": float(grp["mean_node_span"].mean(skipna=True)) if grp["mean_node_span"].notna().any() else np.nan,
                "zero_factor_functions_mean": float(grp["zero_factor_functions"].mean(skipna=True)) if grp["zero_factor_functions"].notna().any() else np.nan,
                "zero_span_functions_mean": float(grp["zero_span_functions"].mean(skipna=True)) if grp["zero_span_functions"].notna().any() else np.nan,
            }
        )
    return pd.DataFrame(summary_rows)


def run_sweep(args: argparse.Namespace) -> None:
    trace_dir = Path(args.source_trace)
    output_dir = Path(args.output_dir)
    ensure_output_dir(output_dir)

    inv_df, mem_df, run_df = load_trace(trace_dir)
    simulation_df = pd.read_csv(args.ca_trace_csv) if args.ca_trace_csv is not None else None
    invocation_columns = get_invocation_columns(inv_df)
    total_before = int(inv_df[invocation_columns].to_numpy().sum())

    rows: list[dict[str, float | int | str]] = []
    seed_end = args.seed_start + args.seed_count
    for seed in range(args.seed_start, seed_end):
        if args.policy in {"round-robin", "both"}:
            append_round_robin_row(
                rows=rows,
                inv_df=inv_df,
                mem_df=mem_df,
                run_df=run_df,
                total_before=total_before,
                invocation_columns=invocation_columns,
                real_nodes=args.real_nodes,
                max_nodes=args.max_nodes,
                seed=seed,
            )
        if args.policy in {"cache-aware", "both"}:
            append_cache_aware_row(
                rows=rows,
                inv_df=inv_df,
                mem_df=mem_df,
                run_df=run_df,
                simulation_df=simulation_df,
                total_before=total_before,
                invocation_columns=invocation_columns,
                real_nodes=args.real_nodes,
                max_nodes=args.max_nodes,
                seed=seed,
                span_stat=args.ca_span_stat,
            )
        if (seed - args.seed_start) % args.progress_every == 0:
            log.info("Completed seed %d", seed)

    results = pd.DataFrame(rows)
    summary = summarize_results(results)
    results_path = output_dir / f"{args.name}_results.csv"
    summary_path = output_dir / f"{args.name}_summary.csv"
    results.to_csv(results_path, index=False)
    summary.to_csv(summary_path, index=False)
    log.info("Wrote seed sweep results to %s", results_path)
    log.info("Wrote seed sweep summary to %s", summary_path)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Experimental trace reduction helper extracted from sampler.")
    subparsers = parser.add_subparsers(dest="cmd", required=True)

    reduce_parser = subparsers.add_parser("reduce", help="Run one reduction and write the reduced trace.")
    reduce_parser.add_argument("-t", "--source-trace", required=True, metavar="path", help="Path to the input trace directory")
    reduce_parser.add_argument("-o", "--output", required=True, metavar="path", help="Output directory for the reduced trace")
    reduce_parser.add_argument("-real", "--real-nodes", required=True, type=int, metavar="integer", help="Number of nodes in the real cluster")
    reduce_parser.add_argument("-max", "--max-nodes", required=True, type=int, metavar="integer", help="Number of nodes in the full trace cluster")
    reduce_parser.add_argument("--seed", required=False, type=int, default=0, metavar="integer", help="Random seed used for reproducible sampling")
    reduce_parser.add_argument("--policy", required=False, default="round-robin", choices=["round-robin", "cache-aware"], help="Reduction policy")
    reduce_parser.add_argument("--ca-trace-csv", required=False, metavar="path", help="Optional timestamp,function,cpu CSV used to derive cache-aware spans")
    reduce_parser.add_argument("--ca-span-stat", required=False, default="max", choices=["max", "p99"], help="Statistic used to derive cache-aware node spans")

    sweep_parser = subparsers.add_parser("sweep", help="Run the same reduction for many seeds and write summary CSVs.")
    sweep_parser.add_argument("-t", "--source-trace", required=True, metavar="path", help="Path to the input trace directory")
    sweep_parser.add_argument("-o", "--output-dir", required=True, metavar="path", help="Directory where results and summary CSVs will be written")
    sweep_parser.add_argument("--name", required=True, metavar="prefix", help="Prefix used for the output CSV filenames")
    sweep_parser.add_argument("-real", "--real-nodes", required=True, type=int, metavar="integer", help="Number of nodes in the real cluster")
    sweep_parser.add_argument("-max", "--max-nodes", required=True, type=int, metavar="integer", help="Number of nodes in the full trace cluster")
    sweep_parser.add_argument("--seed-start", required=False, type=int, default=0, metavar="integer", help="First seed in the sweep")
    sweep_parser.add_argument("--seed-count", required=False, type=int, default=10, metavar="integer", help="Number of consecutive seeds to run")
    sweep_parser.add_argument("--progress-every", required=False, type=int, default=50, metavar="integer", help="Log progress every N seeds")
    sweep_parser.add_argument("--policy", required=False, default="both", choices=["round-robin", "cache-aware", "both"], help="Which policies to include in the sweep")
    sweep_parser.add_argument("--ca-trace-csv", required=False, metavar="path", help="Optional timestamp,function,cpu CSV used to derive cache-aware spans")
    sweep_parser.add_argument("--ca-span-stat", required=False, default="max", choices=["max", "p99"], help="Statistic used to derive cache-aware node spans")

    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()

    if args.cmd == "reduce":
        run_reduce(args)
    else:
        run_sweep(args)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
