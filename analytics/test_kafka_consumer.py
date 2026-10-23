import sqlite3
import json
import unittest
from analytics.kafka_consumer import consume
class Message:
 def __init__(self,payload):self.value=json.dumps(payload).encode()
class Consumer:
 def __init__(self,payloads):self.messages=[Message(x) for x in payloads];self.commits=0
 def __iter__(self):return iter(self.messages)
 def commit(self):self.commits+=1
class ConsumerTests(unittest.TestCase):
 def test_commit_and_idempotency(self):
  event={"event_id":"e1","transfer_id":"t1","event_time":"2026-01-01T00:00:00Z","status":"completed"}
  with sqlite3.connect(":memory:") as db:
   consumer=Consumer([event,event])
   self.assertEqual(consume(consumer,db,limit=2),1)
   self.assertEqual(consumer.commits,2)
 def test_invalid_event_halts(self):
  with sqlite3.connect(":memory:") as db:
   consumer=Consumer([{"status":"invalid"}])
   with self.assertRaises(RuntimeError):consume(consumer,db)
   self.assertEqual(consumer.commits,0)
