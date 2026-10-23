import sqlite3
import unittest
from analytics.pipeline import load
E={"event_id":"e1","transfer_id":"t1","event_time":"2026-10-22T12:00:00Z","status":"completed"}
class TestETL(unittest.TestCase):
 def test_dedup(self):
  with sqlite3.connect(":memory:") as db:
   self.assertEqual(load(db,[E,E])["inserted"],1)
   self.assertEqual(db.execute("SELECT n FROM daily").fetchone()[0],1)
 def test_invalid(self):
  with sqlite3.connect(":memory:") as db:
   self.assertEqual(load(db,[dict(E,status="unknown")])["rejected"],1)
if __name__=="__main__":unittest.main()
