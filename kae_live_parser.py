#!/usr/bin/env python3
"""
KAE Report Parser & Live Data Generator
Watches KAE report markdown files and generates JSON for live visualization.

Usage:
    python kae_live_parser.py --watch
    python kae_live_parser.py --once
"""

import json
import re
import time
import argparse
from pathlib import Path
from datetime import datetime
from collections import defaultdict

def parse_report(filepath):
    """Parse a single KAE report markdown file."""
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()
    
    # Extract run metadata from filename
    # Format: report_SEED_TIMESTAMP.md
    filename = Path(filepath).name
    match = re.match(r'report_(\w+)_(\d{8})_(\d{6})\.md', filename)
    if not match:
        return None
    
    seed = match.group(1)
    date_str = match.group(2)
    time_str = match.group(3)
    
    # Parse cycles
    cycles = re.findall(r'## Cycle (\d+) — (\d{2}:\d{2}:\d{2})\n\*\*Graph:\*\* Nodes: (\d+) \| Edges: (\d+) \| Anomalies: (\d+)', content)
    
    if not cycles:
        return None
    
    last_cycle = cycles[-1]
    run_id = f"run_{seed}_{date_str}_{time_str}"
    
    nodes = []
    
    # Parse each cycle with its concepts
    for cycle_data in cycles:
        cycle_num = int(cycle_data[0])
        
        # Find this cycle's section in the content
        cycle_pattern = rf'## Cycle {cycle_num} —.*?\*\*Emergent concepts:\*\*\n(.*?)(?=\n## Cycle|\Z)'
        cycle_match = re.search(cycle_pattern, content, re.DOTALL)
        
        if not cycle_match:
            continue
            
        section = cycle_match.group(1)
        
        # Parse individual concepts
        # Format: - Concept Name (weight: X.X) [anomaly: X.XX] ⚠
        concept_matches = re.findall(
            r'- (?:\*\*)?(.+?)(?:\*\*)? \(weight: ([\d.]+)\)(?: \[anomaly: ([\d.]+)\])?(?: ⚠)?',
            section
        )
        
        for match in concept_matches:
            name = match[0].strip('*')
            weight = float(match[1])
            anomaly_score = float(match[2]) if match[2] else 0.0
            is_anomaly = anomaly_score > 0.3 or '⚠' in section
            
            nodes.append({
                'name': name,
                'weight': weight,
                'cycle': cycle_num,
                'runId': run_id,
                'isAnomaly': is_anomaly,
                'domain': 'inferred'
            })
    
    return {
        'id': run_id,
        'seed': seed,
        'name': f"{seed.title()} ({date_str})",
        'nodes': int(last_cycle[2]),
        'edges': int(last_cycle[3]),
        'anomalies': int(last_cycle[4]),
        'timestamp': f"{date_str}_{time_str}",
        'concepts': nodes
    }

def find_reports(directory, seed=None):
    """Find all KAE report files."""
    pattern = f"report_{seed}_*.md" if seed else "report_*.md"
    return list(Path(directory).glob(pattern))

def generate_json(reports_dir, output_file, seed_filter=None):
    """Generate JSON data from all reports."""
    report_files = find_reports(reports_dir, seed_filter)
    
    runs = []
    all_nodes = []
    
    for report_file in sorted(report_files):
        run_data = parse_report(report_file)
        if run_data:
            runs.append({
                'id': run_data['id'],
                'name': run_data['name'],
                'nodes': run_data['nodes'],
                'edges': run_data['edges'],
                'anomalies': run_data['anomalies'],
                'seed': run_data['seed']
            })
            all_nodes.extend(run_data['concepts'])
    
    # Group by seed for easier comparison
    by_seed = defaultdict(list)
    for run in runs:
        by_seed[run['seed']].append(run)
    
    data = {
        'timestamp': datetime.now().isoformat(),
        'total_runs': len(runs),
        'seeds': list(by_seed.keys()),
        'runs': runs,
        'nodes': all_nodes
    }
    
    with open(output_file, 'w') as f:
        json.dump(data, f, indent=2)
    
    return len(runs), len(all_nodes)

def watch_and_update(reports_dir, output_file, interval=5, seed_filter=None):
    """Watch for changes and update JSON file."""
    print(f"KAE Live Parser - Watching {reports_dir}")
    print(f"Output: {output_file}")
    print(f"Refresh interval: {interval}s")
    if seed_filter:
        print(f"Filtering seed: {seed_filter}")
    print("\nPress Ctrl+C to stop\n")
    
    try:
        while True:
            runs, nodes = generate_json(reports_dir, output_file, seed_filter)
            timestamp = datetime.now().strftime('%H:%M:%S')
            print(f"[{timestamp}] Updated: {runs} runs, {nodes} concepts")
            time.sleep(interval)
    except KeyboardInterrupt:
        print("\n\nStopped by user")

def main():
    parser = argparse.ArgumentParser(description='Parse KAE reports into JSON for live visualization')
    parser.add_argument('--dir', default='.', help='Directory containing KAE reports')
    parser.add_argument('--output', default='kae_data.json', help='Output JSON file')
    parser.add_argument('--seed', help='Filter by specific seed concept')
    parser.add_argument('--watch', action='store_true', help='Watch mode (continuous updates)')
    parser.add_argument('--interval', type=int, default=5, help='Watch interval in seconds')
    parser.add_argument('--once', action='store_true', help='Generate once and exit')
    
    args = parser.parse_args()
    
    if args.watch:
        watch_and_update(args.dir, args.output, args.interval, args.seed)
    else:
        runs, nodes = generate_json(args.dir, args.output, args.seed)
        print(f"Generated: {runs} runs, {nodes} concepts → {args.output}")

if __name__ == "__main__":
    main()
