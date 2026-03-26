import pandas as pd
from pandas.testing import assert_frame_equal

from reduce import (
    compute_cache_aware_report,
    compute_cache_aware_report_from_simulation,
    compute_round_robin_factor,
    reduce_trace_cache_aware,
    reduce_trace_round_robin,
    thin_invocations_round_robin,
)


def create_trace_tables():
    inv_df = pd.DataFrame(
        {
            "HashApp": ["aa", "ab", "ac"],
            "HashFunction": ["fa", "fb", "fc"],
            "HashOwner": ["oa", "ob", "oc"],
            "Trigger": ["http", "http", "http"],
            "1": [1, 5, 2],
            "2": [0, 5, 2],
            "3": [1, 5, 2],
        }
    )
    mem_df = pd.DataFrame(
        {
            "HashFunction": ["fa", "fb", "fc"],
            "HashApp": ["aa", "ab", "ac"],
            "HashOwner": ["oa", "ob", "oc"],
            "SampleCount": [1, 1, 1],
            "AverageAllocatedMb": [32.0, 64.0, 96.0],
        }
    )
    run_df = pd.DataFrame(
        {
            "HashFunction": ["fa", "fb", "fc"],
            "HashOwner": ["oa", "ob", "oc"],
            "HashApp": ["aa", "ab", "ac"],
            "Average": [10, 20, 30],
            "Count": [1, 1, 1],
            "Minimum": [5, 10, 15],
        }
    )
    return inv_df, mem_df, run_df


def test_compute_round_robin_factor():
    assert compute_round_robin_factor(real_nodes=4, max_nodes=16) == 0.25


def test_thin_invocations_round_robin_is_reproducible():
    inv_df, _, _ = create_trace_tables()
    thinned_df = thin_invocations_round_robin(inv_df=inv_df, factor=0.25, seed=7)

    expected_df = pd.DataFrame(
        {
            "HashApp": ["aa", "ab", "ac"],
            "HashFunction": ["fa", "fb", "fc"],
            "HashOwner": ["oa", "ob", "oc"],
            "Trigger": ["http", "http", "http"],
            "1": [0, 2, 1],
            "2": [0, 0, 0],
            "3": [1, 1, 1],
        }
    )
    assert_frame_equal(thinned_df, expected_df)


def test_reduce_trace_round_robin_filters_zero_invocation_functions():
    inv_df, mem_df, run_df = create_trace_tables()
    reduced_inv_df, reduced_mem_df, reduced_run_df, factor = reduce_trace_round_robin(
        inv_df=inv_df,
        mem_df=mem_df,
        run_df=run_df,
        real_nodes=1,
        max_nodes=4,
        seed=0,
    )

    expected_inv_df = pd.DataFrame(
        {
            "HashApp": ["ab", "ac"],
            "HashFunction": ["fb", "fc"],
            "HashOwner": ["ob", "oc"],
            "Trigger": ["http", "http"],
            "1": [0, 1],
            "2": [0, 1],
            "3": [2, 1],
        }
    )
    expected_mem_df = pd.DataFrame(
        {
            "HashFunction": ["fb", "fc"],
            "HashApp": ["ab", "ac"],
            "HashOwner": ["ob", "oc"],
            "SampleCount": [1, 1],
            "AverageAllocatedMb": [64.0, 96.0],
        }
    )
    expected_run_df = pd.DataFrame(
        {
            "HashFunction": ["fb", "fc"],
            "HashOwner": ["ob", "oc"],
            "HashApp": ["ab", "ac"],
            "Average": [20, 30],
            "Count": [1, 1],
            "Minimum": [10, 15],
        }
    )

    assert factor == 0.25
    assert_frame_equal(reduced_inv_df, expected_inv_df)
    assert_frame_equal(reduced_mem_df, expected_mem_df)
    assert_frame_equal(reduced_run_df, expected_run_df)


def test_compute_cache_aware_report_is_reproducible():
    inv_df, _, run_df = create_trace_tables()
    run_df = run_df.copy()
    run_df["Average"] = [30000, 18000, 45000]

    report_df, real_node_ids = compute_cache_aware_report(
        inv_df=inv_df,
        run_df=run_df,
        real_nodes=1,
        max_nodes=4,
        seed=0,
    )

    expected_report = pd.DataFrame(
        {
            "HashOwner": ["oa", "ob", "oc"],
            "HashApp": ["aa", "ab", "ac"],
            "HashFunction": ["fa", "fb", "fc"],
            "Trigger": ["http", "http", "http"],
            "peak_concurrency": [0.5, 1.5, 1.5],
            "node_span": [1, 2, 2],
            "node_overlap": [0, 1, 1],
            "placement_factor": [0.0, 0.5, 0.5],
            "invocations_before": [2, 15, 6],
        }
    )

    assert_frame_equal(report_df, expected_report)
    assert real_node_ids.tolist() == [3]


def test_reduce_trace_cache_aware_filters_zero_invocation_functions():
    inv_df, mem_df, run_df = create_trace_tables()
    run_df = run_df.copy()
    run_df["Average"] = [30000, 18000, 45000]

    reduced_inv_df, reduced_mem_df, reduced_run_df, report_df, real_node_ids = reduce_trace_cache_aware(
        inv_df=inv_df,
        mem_df=mem_df,
        run_df=run_df,
        real_nodes=1,
        max_nodes=4,
        seed=0,
    )

    expected_inv_df = pd.DataFrame(
        {
            "HashApp": ["ab", "ac"],
            "HashFunction": ["fb", "fc"],
            "HashOwner": ["ob", "oc"],
            "Trigger": ["http", "http"],
            "1": [3, 2],
            "2": [4, 1],
            "3": [1, 1],
        }
    )
    expected_mem_df = pd.DataFrame(
        {
            "HashFunction": ["fb", "fc"],
            "HashApp": ["ab", "ac"],
            "HashOwner": ["ob", "oc"],
            "SampleCount": [1, 1],
            "AverageAllocatedMb": [64.0, 96.0],
        }
    )
    expected_run_df = pd.DataFrame(
        {
            "HashFunction": ["fb", "fc"],
            "HashOwner": ["ob", "oc"],
            "HashApp": ["ab", "ac"],
            "Average": [18000, 45000],
            "Count": [1, 1],
            "Minimum": [10, 15],
        }
    )

    assert real_node_ids.tolist() == [3]
    assert report_df["kept"].tolist() == [False, True, True]
    assert report_df["invocations_after"].tolist() == [0, 8, 4]
    assert_frame_equal(reduced_inv_df, expected_inv_df)
    assert_frame_equal(reduced_mem_df, expected_mem_df)
    assert_frame_equal(reduced_run_df, expected_run_df)


def test_compute_cache_aware_report_from_simulation_uses_function_ids():
    inv_df, _, _ = create_trace_tables()
    simulation_df = pd.DataFrame(
        {
            "timestamp": [0, 1, 2, 0, 1],
            "function": [0, 0, 0, 2, 2],
            "cpu": [0.2, 0.0, 1.8, 2.0, 2.0],
        }
    )

    report_df, real_node_ids = compute_cache_aware_report_from_simulation(
        inv_df=inv_df,
        simulation_df=simulation_df,
        real_nodes=1,
        max_nodes=4,
        seed=0,
        span_stat="max",
    )

    expected_spans = [2, 0, 2]
    expected_metric_values = [1.8, 0.0, 2.0]
    expected_factors = [0.0, 0.0, 0.5]

    assert real_node_ids.tolist() == [3]
    assert report_df["trace_function_id"].tolist() == [0, 1, 2]
    assert report_df["timeline_stat_value"].tolist() == expected_metric_values
    assert report_df["node_span"].tolist() == expected_spans
    assert report_df["placement_factor"].tolist() == expected_factors


def run_all_tests():
    test_compute_round_robin_factor()
    test_thin_invocations_round_robin_is_reproducible()
    test_reduce_trace_round_robin_filters_zero_invocation_functions()
    test_compute_cache_aware_report_is_reproducible()
    test_reduce_trace_cache_aware_filters_zero_invocation_functions()
    test_compute_cache_aware_report_from_simulation_uses_function_ids()


if __name__ == "__main__":
    run_all_tests()
    print("all_exp_sampler_tests_passed")
