# Activity Log Report Generator

## Overview

`generate_activity_report.py` is a Python script that parses markdown activity logs and generates comprehensive HTML reports with visual heatmaps and statistics.

## Usage

```bash
python3 generate_activity_report.py <input.md> [output.html]
```

### Examples

```bash
# Generate report with default output name (input file name + .html)
python3 generate_activity_report.py activity_log.md

# Specify custom output file
python3 generate_activity_report.py activity_log.md report.html
```

## Input Format

The script expects a markdown file with the following structure:

```
Person Name action to Category Name
Date (e.g., "Oct 23, 2025")
Document Name
```

### Example Input

```
Kevin Schneider updated to RC - 1˸10 4WD Buggy
Oct 23, 2025
CFG - Center Slipper ASSY
Kevin Schneider updated to RC - 1˸10 4WD Buggy
Oct 23, 2025
Center Slipper Outdrive
```

## Output

The script generates an HTML file containing:

1. **Summary Statistics**
   - Total activities
   - Date range
   - Unique dates
   - Average activities per day

2. **Activity Heatmap**
   - Visual color-coded heatmap showing activity intensity by day
   - Scale from light to dark indicating activity levels

3. **Top 10 Documents**
   - Most frequently updated/uploaded documents

4. **Activity by Category**
   - Breakdown of activities by folder/category

5. **Activity by Type**
   - Count of updates vs uploads

## Features

- ✅ Automatic date parsing and sorting
- ✅ Color-coded activity heatmap
- ✅ Statistical summaries
- ✅ Category and document analysis
- ✅ No external dependencies (uses only Python standard library)

## Requirements

- Python 3.6+
- No external packages required

## Example Output

The generated HTML includes:
- Modern, responsive design
- Interactive heatmap visualization
- Clean statistics dashboard
- Comprehensive activity breakdowns

