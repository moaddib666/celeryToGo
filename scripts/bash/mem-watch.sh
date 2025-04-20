#!/bin/bash

# Parse command line arguments
while getopts "p:s:" opt; do
  case $opt in
    p) PID=$OPTARG ;;
    s) SECONDS_INTERVAL=$OPTARG ;;
    *) echo "Usage: $0 -p PID -s SECONDS_INTERVAL" >&2
       exit 1 ;;
  esac
done

# Check if required parameters are provided
if [ -z "$PID" ] || [ -z "$SECONDS_INTERVAL" ]; then
  echo "Usage: $0 -p PID -s SECONDS_INTERVAL"
  exit 1
fi

# Check if PID exists
if ! ps -p $PID > /dev/null; then
  echo "Error: Process with PID $PID does not exist."
  exit 1
fi

# Function to get memory usage (in KB) for a single process
get_proc_memory() {
  local pid=$1
  local rss=$(ps -o rss= -p $pid 2>/dev/null || echo 0)
  echo $rss
}

# Function to calculate total memory usage for a process and all its children
calculate_total_memory() {
  local parent_pid=$1
  local parent_mem=$(get_proc_memory $parent_pid)

  # Get all child PIDs recursively
  local all_children=$(pgrep -P $parent_pid)
  local child_pids=()
  local child_mems=()
  local total_child_mem=0

  # First level children
  for child in $all_children; do
    # Get grandchildren recursively
    local descendants=$(pstree -p $child | grep -o '([0-9]\+)' | grep -o '[0-9]\+' | tr '\n' ' ')
    for descendant in $descendants; do
      if [ "$descendant" != "$child" ]; then
        all_children="$all_children $descendant"
      fi
    done
  done

  # Remove duplicates
  all_children=$(echo "$all_children" | tr ' ' '\n' | sort -u | tr '\n' ' ')

  # Calculate memory for each child
  for child in $all_children; do
    local child_mem=$(get_proc_memory $child)
    child_pids+=($child)
    child_mems+=($child_mem)
    total_child_mem=$((total_child_mem + child_mem))
  done

  # Total memory including parent and all children
  local total_mem=$((parent_mem + total_child_mem))

  # Print summary
  echo "============================================================"
  date +"%Y-%m-%d %H:%M:%S"
  echo "Memory usage summary for PID $parent_pid and children"
  echo "============================================================"
  echo "Parent process ($parent_pid): $parent_mem KB"
  echo "All children combined: $total_child_mem KB"
  echo "Total memory usage: $total_mem KB ($(echo "scale=2; $total_mem/1024" | bc) MB)"
  echo "============================================================"
  echo "Child processes details:"
  echo "PID     MEMORY(KB)  COMMAND"

  # Get command name for parent
  local parent_cmd=$(ps -o comm= -p $parent_pid 2>/dev/null || echo "unknown")

  # Print details for children sorted by memory usage (if any)
  if [ ${#child_pids[@]} -gt 0 ]; then
    # Create a temporary file for sorting
    local tmp_file=$(mktemp)

    for i in ${!child_pids[@]}; do
      local cmd=$(ps -o comm= -p ${child_pids[$i]} 2>/dev/null || echo "unknown")
      echo "${child_pids[$i]} ${child_mems[$i]} $cmd" >> $tmp_file
    done

    # Sort by memory usage (descending)
    sort -k2 -nr $tmp_file | while read pid mem cmd; do
      printf "%-7s %-11s %s\n" $pid $mem "$cmd"
    done

    rm $tmp_file
  else
    echo "No child processes found."
  fi
}

# Main monitoring loop
echo "Starting memory monitoring for PID $PID and all children. Updates every $SECONDS_INTERVAL seconds."
echo "Press Ctrl+C to stop monitoring."

while true; do
  clear
  calculate_total_memory $PID
  sleep $SECONDS_INTERVAL
done