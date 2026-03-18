#!/bin/bash
TARGET_FILE="targets.json"
rm -f $TARGET_FILE

echo "Generating $TARGET_FILE..."
for i in {1..10000}; do
  src="user$((RANDOM % 100))"
  dst="user$((RANDOM % 100))"
  amt=$((RANDOM % 100 + 1))
  txnid="txn-v-$i-$RANDOM"
  
  # Base64 encode the body as required by vegeta JSON format
  body=$(echo -n "{\"TxnID\": \"$txnid\", \"Source\": \"$src\", \"Destination\": \"$dst\", \"Amount\": $amt}" | base64)
  
  echo "{\"method\": \"POST\", \"url\": \"http://localhost:8000/submit\", \"header\": {\"Content-Type\": [\"application/json\"], \"Authorization\": [\"Bearer dummy-token\"]}, \"body\": \"$body\"}" >> $TARGET_FILE
done

echo "Running Vegeta attack at 500 req/s for 30s..."
vegeta attack -format=json -rate=500 -duration=30s -targets=$TARGET_FILE | tee results.bin | vegeta report

echo ""
echo "Outputting Latency Histogram:"
vegeta report -type=hist[0,50ms,100ms,200ms,500ms,1s] results.bin
