"""At-least-once Kafka metadata ingestion to SQLite with event-id deduplication."""
import argparse
import json
import sqlite3
from analytics.pipeline import load
def consume(consumer,db,limit=100):
    accepted=0
    for message in consumer:
        try:
            event=json.loads(message.value.decode("utf8"))
            result=load(db,[event])
            if result["rejected"]:
                # Do not commit malformed events: operator should configure a DLQ.
                raise ValueError("rejected event; DLQ required")
            consumer.commit()
            accepted+=result["inserted"]
        except (ValueError,UnicodeDecodeError) as exc:
            raise RuntimeError("message processing halted; configure a DLQ") from exc
        if accepted>=limit:break
    return accepted
def main():
    from kafka import KafkaConsumer
    p=argparse.ArgumentParser()
    p.add_argument("--brokers",default="localhost:9092")
    p.add_argument("--topic",default="pixelgo.transfer.events")
    p.add_argument("--db",required=True)
    p.add_argument("--limit",type=int,default=100)
    a=p.parse_args()
    c=KafkaConsumer(a.topic,bootstrap_servers=a.brokers.split(","),group_id="pixelgo-analytics",
      enable_auto_commit=False,auto_offset_reset="earliest")
    try:
        with sqlite3.connect(a.db) as db:print(consume(c,db,a.limit))
    finally:c.close()
if __name__=="__main__":main()
