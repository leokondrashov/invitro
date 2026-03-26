from __future__ import annotations

from typing import Tuple

import numpy as np
import pandas as pd


TRACE_METADATA_COLUMNS = {"HashOwner", "HashApp", "HashFunction", "Trigger"}
SIMULATION_TRACE_COLUMNS = {"timestamp", "function", "cpu"}


def validate_cluster_sizes(real_nodes: int, max_nodes: int) -> None:
    if max_nodes <= 0:
        raise ValueError("max_nodes must be positive")
    if real_nodes < 0:
        raise ValueError("real_nodes must be non-negative")
    if real_nodes > max_nodes:
        raise ValueError("real_nodes cannot exceed max_nodes")


def get_invocation_columns(inv_df: pd.DataFrame) -> list[str]:
    invocation_columns = [column for column in inv_df.columns if column not in TRACE_METADATA_COLUMNS]
    if not invocation_columns:
        raise ValueError("Invocation trace must contain at least one minute column")
    return invocation_columns


def compute_round_robin_factor(real_nodes: int, max_nodes: int) -> float:
    validate_cluster_sizes(real_nodes=real_nodes, max_nodes=max_nodes)
    return real_nodes / max_nodes


def thin_invocations_round_robin(inv_df: pd.DataFrame, factor: float, seed: int | None = None) -> pd.DataFrame:
    if factor < 0.0 or factor > 1.0:
        raise ValueError("round-robin factor must be within [0, 1]")

    invocation_columns = get_invocation_columns(inv_df)
    counts = inv_df[invocation_columns].to_numpy(dtype=np.int64, copy=True)
    rng = np.random.default_rng(seed)
    thinned_counts = rng.binomial(counts, factor)

    reduced_inv_df = inv_df.copy()
    reduced_inv_df.loc[:, invocation_columns] = pd.DataFrame(
        thinned_counts,
        columns=invocation_columns,
        index=reduced_inv_df.index,
    )
    return reduced_inv_df


def thin_invocations_per_function(inv_df: pd.DataFrame, factors: np.ndarray, seed: int | None = None) -> pd.DataFrame:
    if len(factors) != len(inv_df):
        raise ValueError("Expected one thinning factor per function")
    if np.any(factors < 0.0) or np.any(factors > 1.0):
        raise ValueError("Function-specific thinning factors must be within [0, 1]")

    invocation_columns = get_invocation_columns(inv_df)
    counts = inv_df[invocation_columns].to_numpy(dtype=np.int64, copy=True)
    probabilities = np.asarray(factors, dtype=np.float64).reshape(-1, 1)
    rng = np.random.default_rng(seed)
    thinned_counts = rng.binomial(counts, probabilities)

    reduced_inv_df = inv_df.copy()
    reduced_inv_df.loc[:, invocation_columns] = pd.DataFrame(
        thinned_counts,
        columns=invocation_columns,
        index=reduced_inv_df.index,
    )
    return reduced_inv_df


def _reindex_by_hash(df: pd.DataFrame, hash_functions: list[str]) -> pd.DataFrame:
    if len(hash_functions) == 0:
        return df.iloc[0:0].copy().reset_index(drop=True)
    return df.set_index("HashFunction").loc[hash_functions].reset_index()


def filter_zero_invocation_functions(
    inv_df: pd.DataFrame,
    mem_df: pd.DataFrame,
    run_df: pd.DataFrame,
) -> Tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    invocation_columns = get_invocation_columns(inv_df)
    keep_mask = inv_df[invocation_columns].sum(axis=1) > 0

    reduced_inv_df = inv_df.loc[keep_mask].reset_index(drop=True)
    kept_hashes = reduced_inv_df["HashFunction"].tolist()
    reduced_mem_df = _reindex_by_hash(mem_df, kept_hashes)
    reduced_run_df = _reindex_by_hash(run_df, kept_hashes)

    return reduced_inv_df, reduced_mem_df, reduced_run_df


def estimate_peak_concurrency(inv_df: pd.DataFrame, run_df: pd.DataFrame) -> np.ndarray:
    invocation_columns = get_invocation_columns(inv_df)
    aligned_run_df = _reindex_by_hash(run_df, inv_df["HashFunction"].tolist())
    avg_duration_ms = aligned_run_df["Average"].to_numpy(dtype=np.float64)
    invocation_counts = inv_df[invocation_columns].to_numpy(dtype=np.float64, copy=False)

    return (invocation_counts * avg_duration_ms[:, np.newaxis] / 60000.0).max(axis=1)


def estimate_cache_aware_node_spans(inv_df: pd.DataFrame, run_df: pd.DataFrame, max_nodes: int) -> np.ndarray:
    peak_concurrency = estimate_peak_concurrency(inv_df=inv_df, run_df=run_df)
    node_spans = np.ceil(np.maximum(peak_concurrency, 1.0)).astype(np.int64)
    return np.clip(node_spans, 1, max_nodes)


def sample_real_nodes(real_nodes: int, max_nodes: int, seed: int | None = None) -> np.ndarray:
    validate_cluster_sizes(real_nodes=real_nodes, max_nodes=max_nodes)
    if real_nodes == 0:
        return np.array([], dtype=np.int64)

    rng = np.random.default_rng(seed)
    return np.sort(rng.choice(max_nodes, size=real_nodes, replace=False))


def sample_node_overlaps(node_spans: np.ndarray, real_nodes: int, max_nodes: int, seed: int | None = None) -> tuple[np.ndarray, np.ndarray]:
    real_node_ids = sample_real_nodes(real_nodes=real_nodes, max_nodes=max_nodes, seed=seed)
    real_node_set = set(real_node_ids.tolist())

    rng = np.random.default_rng(seed)
    if real_nodes > 0:
        # Consume the real-node sample first so placement stays reproducible with the same seed.
        rng.choice(max_nodes, size=real_nodes, replace=False)

    overlaps = np.zeros(len(node_spans), dtype=np.int64)
    for idx, node_span in enumerate(np.asarray(node_spans, dtype=np.int64)):
        if node_span <= 0:
            overlaps[idx] = 0
            continue
        if real_nodes == 0:
            overlaps[idx] = 0
            continue
        if node_span >= max_nodes:
            overlaps[idx] = real_nodes
            continue

        function_nodes = rng.choice(max_nodes, size=int(node_span), replace=False)
        overlaps[idx] = len(real_node_set.intersection(function_nodes.tolist()))

    return real_node_ids, overlaps


def finalize_cache_aware_report(
    report_df: pd.DataFrame,
    inv_df: pd.DataFrame,
    real_nodes: int,
    max_nodes: int,
    seed: int | None = None,
) -> tuple[pd.DataFrame, np.ndarray]:
    real_node_ids, overlaps = sample_node_overlaps(
        node_spans=report_df["node_span"].to_numpy(dtype=np.int64),
        real_nodes=real_nodes,
        max_nodes=max_nodes,
        seed=seed,
    )
    report_df["node_overlap"] = overlaps

    node_spans = report_df["node_span"].to_numpy(dtype=np.int64)
    placement_factors = np.divide(
        overlaps,
        node_spans,
        out=np.zeros_like(overlaps, dtype=np.float64),
        where=node_spans > 0,
    )
    report_df["placement_factor"] = placement_factors

    invocation_columns = get_invocation_columns(inv_df)
    report_df["invocations_before"] = inv_df[invocation_columns].sum(axis=1).to_numpy(dtype=np.int64)
    return report_df, real_node_ids


def compute_cache_aware_report(
    inv_df: pd.DataFrame,
    run_df: pd.DataFrame,
    real_nodes: int,
    max_nodes: int,
    seed: int | None = None,
) -> tuple[pd.DataFrame, np.ndarray]:
    validate_cluster_sizes(real_nodes=real_nodes, max_nodes=max_nodes)

    report_df = inv_df[["HashOwner", "HashApp", "HashFunction", "Trigger"]].copy()
    report_df["peak_concurrency"] = estimate_peak_concurrency(inv_df=inv_df, run_df=run_df)
    report_df["node_span"] = estimate_cache_aware_node_spans(inv_df=inv_df, run_df=run_df, max_nodes=max_nodes)

    return finalize_cache_aware_report(
        report_df=report_df,
        inv_df=inv_df,
        real_nodes=real_nodes,
        max_nodes=max_nodes,
        seed=seed,
    )


def compute_simulation_timeline_stat(
    simulation_df: pd.DataFrame,
    function_count: int,
    span_stat: str,
) -> tuple[np.ndarray, int]:
    if span_stat not in {"max", "p99"}:
        raise ValueError("span_stat must be either 'max' or 'p99'")

    missing_columns = SIMULATION_TRACE_COLUMNS.difference(simulation_df.columns)
    if missing_columns:
        raise ValueError(f"Simulation trace is missing required columns: {sorted(missing_columns)}")

    if simulation_df.empty:
        return np.zeros(function_count, dtype=np.float64), 1

    grouped_df = (
        simulation_df.loc[:, ["timestamp", "function", "cpu"]]
        .groupby(["function", "timestamp"], as_index=False)["cpu"]
        .sum()
    )
    function_ids = grouped_df["function"].to_numpy(dtype=np.int64)
    timestamps = grouped_df["timestamp"].to_numpy(dtype=np.int64)

    if function_ids.min() < 0:
        raise ValueError("Simulation function ids must be non-negative")
    if function_ids.max() >= function_count:
        raise ValueError("Simulation function ids exceed sampled trace size")
    if timestamps.min() < 0:
        raise ValueError("Simulation timestamps must be non-negative")

    horizon = int(timestamps.max()) + 1
    timeline = np.zeros((function_count, horizon), dtype=np.float64)
    timeline[function_ids, timestamps] = grouped_df["cpu"].to_numpy(dtype=np.float64)

    if span_stat == "max":
        return timeline.max(axis=1), horizon
    return np.quantile(timeline, 0.99, axis=1), horizon


def compute_cache_aware_report_from_simulation(
    inv_df: pd.DataFrame,
    simulation_df: pd.DataFrame,
    real_nodes: int,
    max_nodes: int,
    seed: int | None = None,
    span_stat: str = "p99",
) -> tuple[pd.DataFrame, np.ndarray]:
    validate_cluster_sizes(real_nodes=real_nodes, max_nodes=max_nodes)

    stat_values, horizon = compute_simulation_timeline_stat(
        simulation_df=simulation_df,
        function_count=len(inv_df),
        span_stat=span_stat,
    )
    report_df = inv_df[["HashOwner", "HashApp", "HashFunction", "Trigger"]].copy().reset_index(drop=True)
    report_df["trace_function_id"] = report_df.index
    report_df["timeline_horizon"] = horizon
    report_df["timeline_stat"] = span_stat
    report_df["timeline_stat_value"] = stat_values
    report_df["node_span"] = np.where(
        stat_values > 0.0,
        np.clip(np.ceil(stat_values), 1, max_nodes),
        0,
    ).astype(np.int64)

    return finalize_cache_aware_report(
        report_df=report_df,
        inv_df=inv_df,
        real_nodes=real_nodes,
        max_nodes=max_nodes,
        seed=seed,
    )


def reduce_trace_cache_aware(
    inv_df: pd.DataFrame,
    mem_df: pd.DataFrame,
    run_df: pd.DataFrame,
    real_nodes: int,
    max_nodes: int,
    seed: int | None = None,
    simulation_df: pd.DataFrame | None = None,
    span_stat: str = "max",
) -> tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame, pd.DataFrame, np.ndarray]:
    if simulation_df is None:
        report_df, real_node_ids = compute_cache_aware_report(
            inv_df=inv_df,
            run_df=run_df,
            real_nodes=real_nodes,
            max_nodes=max_nodes,
            seed=seed,
        )
    else:
        report_df, real_node_ids = compute_cache_aware_report_from_simulation(
            inv_df=inv_df,
            simulation_df=simulation_df,
            real_nodes=real_nodes,
            max_nodes=max_nodes,
            seed=seed,
            span_stat=span_stat,
        )

    thinning_seed = None if seed is None else seed + 1
    reduced_inv_df = thin_invocations_per_function(
        inv_df=inv_df,
        factors=report_df["placement_factor"].to_numpy(dtype=np.float64),
        seed=thinning_seed,
    )

    invocation_columns = get_invocation_columns(inv_df)
    report_df["invocations_after"] = reduced_inv_df[invocation_columns].sum(axis=1).to_numpy(dtype=np.int64)
    report_df["kept"] = report_df["invocations_after"] > 0

    reduced_inv_df, reduced_mem_df, reduced_run_df = filter_zero_invocation_functions(
        inv_df=reduced_inv_df,
        mem_df=mem_df,
        run_df=run_df,
    )
    return reduced_inv_df, reduced_mem_df, reduced_run_df, report_df, real_node_ids


def reduce_trace_round_robin(
    inv_df: pd.DataFrame,
    mem_df: pd.DataFrame,
    run_df: pd.DataFrame,
    real_nodes: int,
    max_nodes: int,
    seed: int | None = None,
) -> Tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame, float]:
    factor = compute_round_robin_factor(real_nodes=real_nodes, max_nodes=max_nodes)
    reduced_inv_df = thin_invocations_round_robin(inv_df=inv_df, factor=factor, seed=seed)
    reduced_inv_df, reduced_mem_df, reduced_run_df = filter_zero_invocation_functions(
        inv_df=reduced_inv_df,
        mem_df=mem_df,
        run_df=run_df,
    )
    return reduced_inv_df, reduced_mem_df, reduced_run_df, factor
