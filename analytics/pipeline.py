"""Idempotent transaction-status metadata ingest, decoupled from production."""
import json
import sqlite3
from datetime import datetime,timezone
VALID={"created","queued","processing","completed","failed","cancelled"}
SCHEMA="""CREATE TABLE IF NOT EXISTS events(
 event_id TEXT PRIMARY KEY,transfer_id TEXT NOT NULL,day_utc TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('created','queued','processing','completed','failed','cancelled')));
CREATE VIEW IF NOT EXISTS daily AS SELECT day_utc,status,COUNT(*) n
 FROM events GROUP BY day_utc,status;"""
def transform(e):
    if e["status"] not in VALID or not str(e["event_id"]).strip() or not str(e["transfer_id"]).strip():
        raise ValueError("invalid transfer")
    t=datetime.fromisoformat(e["event_time"].replace("Z","+00:00"))
    if t.tzinfo is None:raise ValueError("timezone required")
    return str(e["event_id"]),str(e["transfer_id"]),t.astimezone(timezone.utc).date().isoformat(),e["status"]
def load(db,events):
    db.executescript(SCHEMA)
    inserted=rejected=0
    for e in events:
        try:inserted+=db.execute("INSERT OR IGNORE INTO events VALUES(?,?,?,?)",transform(e)).rowcount
        except (KeyError,ValueError,TypeError):rejected+=1
    db.commit()
    return {"inserted":inserted,"rejected":rejected}
def main():
    import argparse
    p=argparse.ArgumentParser()
    p.add_argument("--input",required=True)
    p.add_argument("--db",default="transfer_analytics.sqlite")
    a=p.parse_args()
    with sqlite3.connect(a.db) as db,open(a.input,encoding="utf8") as f:
        print(json.dumps(load(db,(json.loads(line) for line in f if line.strip()))))
if __name__=="__main__":main()
