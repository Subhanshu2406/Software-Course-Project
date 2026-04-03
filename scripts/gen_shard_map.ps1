# PowerShell version of gen_shard_map.sh
# Generates config/shard_map.json from .env variables

# Load .env file
$env_file = ".\.env"
$NUM_SHARDS = 3
$TOTAL_PARTITIONS = 30

if (Test-Path $env_file) {
    Get-Content $env_file | ForEach-Object {
        if ($_ -match "^([^#=]+)=(.*)$") {
            $key = $matches[1].Trim()
            $value = $matches[2].Trim()
            
            if ($key -eq "NUM_SHARDS") {
                $NUM_SHARDS = [int]$value
            }
            elseif ($key -eq "TOTAL_PARTITIONS") {
                $TOTAL_PARTITIONS = [int]$value
            }
        }
    }
}

Write-Host "NUM_SHARDS=$NUM_SHARDS, TOTAL_PARTITIONS=$TOTAL_PARTITIONS" -ForegroundColor Cyan

# Validate
if ($TOTAL_PARTITIONS % $NUM_SHARDS -ne 0) {
    Write-Host "ERROR: TOTAL_PARTITIONS ($TOTAL_PARTITIONS) must be divisible by NUM_SHARDS ($NUM_SHARDS)" -ForegroundColor Red
    exit 1
}

$PARTITIONS_PER_SHARD = [int]($TOTAL_PARTITIONS / $NUM_SHARDS)

# Build partition map
$partitions = @{}
$port = 8081

for ($i = 0; $i -lt $TOTAL_PARTITIONS; $i++) {
    $shard_index = [int]($i / $PARTITIONS_PER_SHARD)
    $shard_id = "shard$($shard_index + 1)"
    $shard_port = $port + $shard_index
    
    $partitions[$i.ToString()] = @{
        shard_id = $shard_id
        address = "$($shard_id):$shard_port"
        role = "PRIMARY"
    }
}

# Build JSON
$shard_map = @{ partitions = $partitions }
$json = $shard_map | ConvertTo-Json -Depth 10

# Ensure config directory exists
if (-not (Test-Path "config")) {
    New-Item -ItemType Directory -Path "config" | Out-Null
}

# Write to file WITHOUT BOM (important for JSON parsing)
$bytes = [System.Text.Encoding]::UTF8.GetBytes($json)
[System.IO.File]::WriteAllBytes("config/shard_map.json", $bytes)

Write-Host "Generated config/shard_map.json with $TOTAL_PARTITIONS partitions across $NUM_SHARDS shards" -ForegroundColor Green
