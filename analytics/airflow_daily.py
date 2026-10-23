"""Scheduled analytics job; requires Airflow runtime and mounted export."""
from datetime import datetime
from airflow import DAG
from airflow.operators.bash import BashOperator
with DAG("pixelgo_transfer_analytics",start_date=datetime(2026,1,1),
         schedule="@daily",catchup=False,max_active_runs=1,tags=["pixelgo"]) as dag:
    aggregate=BashOperator(task_id="aggregate",bash_command=(
        "cd \"$PIXELGO_REPO_ROOT\" && python3 -m analytics.pipeline "
        "--input \"$PIXELGO_EVENT_EXPORT\" --db \"$PIXELGO_ANALYTICS_DB\""))
