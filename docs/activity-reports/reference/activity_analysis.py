#!/usr/bin/env python3
from collections import Counter
from datetime import datetime
from io import StringIO
import re

# Parse the activity log
with open('/Users/schneik/Documents/Kevin Schneider updated to RC - 1˸10 4WD.md', 'r') as f:
    lines = f.readlines()

activities = []
i = 0
while i < len(lines):
    if 'Kevin Schneider' in lines[i]:
        # Parse the action line: "Kevin Schneider updated to RC - 1˸10 4WD Buggy"
        action_line = lines[i].strip()
        action_type = 'updated' if 'updated' in action_line else 'uploaded'
        
        # Extract category/folder
        if ' to ' in action_line:
            category = action_line.split(' to ')[1]
        else:
            category = 'Unknown'
        
        # Get the date
        if i + 1 < len(lines):
            date_str = lines[i + 1].strip()
            try:
                date_obj = datetime.strptime(date_str, '%b %d, %Y')
            except:
                i += 3
                continue
        else:
            break
            
        # Get the document name
        if i + 2 < len(lines):
            doc_name = lines[i + 2].strip()
            # Skip empty or malformed entries
            if doc_name and doc_name != action_line:
                activities.append({
                    'date': date_obj,
                    'date_str': date_str,
                    'category': category,
                    'document': doc_name,
                    'action': action_type
                })
        i += 3
    else:
        i += 1

# Create summary statistics
total_activities = len(activities)
dates = [a['date'] for a in activities]
min_date = min(dates)
max_date = max(dates)
unique_dates = len(set(dates))

# Top 10 documents by activity count
doc_counter = Counter([a['document'] for a in activities])
top_docs = doc_counter.most_common(10)

# Activities by day for heatmap data
daily_counter = Counter([a['date_str'] for a in activities])

# Categories breakdown
category_counter = Counter([a['category'] for a in activities])

# Generate HTML with heatmap visualization
html_content = """<!DOCTYPE html>
<html>
<head>
    <title>Activity Log Analysis</title>
    <style>
        body {{ font-family: Arial, sans-serif; margin: 40px; background: #f5f5f5; }}
        .container {{ background: white; padding: 30px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }}
        h1 {{ color: #333; border-bottom: 3px solid #4CAF50; padding-bottom: 10px; }}
        h2 {{ color: #666; margin-top: 30px; }}
        .summary {{ background: #e8f5e9; padding: 20px; border-radius: 5px; margin: 20px 0; }}
        .heatmap {{ margin: 30px 0; }}
        .heatmap-row {{ display: flex; margin: 5px 0; }}
        .heatmap-date {{ width: 120px; text-align: right; padding-right: 15px; font-size: 12px; }}
        .heatmap-cell {{ width: 30px; height: 30px; margin: 2px; border-radius: 3px; display: inline-block; text-align: center; line-height: 30px; font-size: 10px; color: #fff; font-weight: bold; }}
        .scale {{ display: flex; align-items: center; margin-top: 15px; }}
        .scale-label {{ margin-right: 10px; font-size: 12px; }}
        .scale-item {{ width: 30px; height: 20px; display: inline-block; margin: 0 2px; border-radius: 3px; }}
        .top-docs {{ background: #f5f5f5; padding: 20px; border-radius: 5px; }}
        .doc-item {{ padding: 10px; margin: 5px 0; background: white; border-left: 4px solid #4CAF50; }}
        .category-list {{ margin-left: 20px; }}
        .category-item {{ padding: 5px 0; }}
    </style>
</head>
<body>
    <div class="container">
        <h1>Activity Log Analysis</h1>
        
        <div class="summary">
            <h2>Summary</h2>
            <p><strong>Total Activities:</strong> {total_activities}</p>
            <p><strong>Date Range:</strong> {date_range}</p>
            <p><strong>Unique Dates:</strong> {unique_dates}</p>
            <p><strong>Average Activities per Day:</strong> {avg_per_day:.1f}</p>
        </div>
        
        <h2>Activity Heatmap by Day</h2>
        <div class="heatmap" id="heatmap">
            {heatmap_html}
        </div>
        <div class="scale">
            <span class="scale-label">Scale:</span>
            <span class="scale-item" style="background: #fff7ec;"></span>
            <span class="scale-item" style="background: #fec981;"></span>
            <span class="scale-item" style="background: #fd8d3c;"></span>
            <span class="scale-item" style="background: #e6550d;"></span>
            <span class="scale-item" style="background: #a63603;"></span>
            <span style="margin-left: 10px; font-size: 11px; color: #666;">Less → More Activities</span>
        </div>
        
        <h2>Top 10 Most Active Documents</h2>
        <div class="top-docs">
            {top_docs_html}
        </div>
        
        <h2>Activity by Category</h2>
        {category_html}
    </div>
</body>
</html>
"""

# Generate heatmap
max_count = max(daily_counter.values()) if daily_counter else 1
def get_color(count):
    if count == 0:
        return '#ffffff'
    elif count <= 2:
        return '#fff7ec'
    elif count <= 5:
        return '#fec981'
    elif count <= 8:
        return '#fd8d3c'
    elif count <= 12:
        return '#e6550d'
    else:
        return '#a63603'

# Sort dates chronologically
sorted_dates = sorted(daily_counter.items(), key=lambda x: datetime.strptime(x[0], '%b %d, %Y'))
heatmap_html = ""
for date, count in sorted_dates:
    color = get_color(count)
    heatmap_html += f"""
    <div class="heatmap-row">
        <span class="heatmap-date">{date}</span>
        <span class="heatmap-cell" style="background: {color};">{count}</span>
    </div>"""

# Generate top documents list
top_docs_html = ""
for i, (doc, count) in enumerate(top_docs, 1):
    top_docs_html += f"""
    <div class="doc-item">
        <strong>{i}. {doc}</strong> - {count} activity(ies)
    </div>"""

# Generate category breakdown
category_html = "<div class='category-list'>"
for cat, count in category_counter.most_common(10):
    category_html += f"<div class='category-item'><strong>{cat}:</strong> {count} activities</div>"
category_html += "</div>"

# Fill in the HTML template
date_range = f"{min_date.strftime('%B %d, %Y')} to {max_date.strftime('%B %d, %Y')}"
avg_per_day = total_activities / unique_dates if unique_dates > 0 else 0

html_content = html_content.format(
    total_activities=total_activities,
    date_range=date_range,
    unique_dates=unique_dates,
    avg_per_day=avg_per_day,
    heatmap_html=heatmap_html,
    top_docs_html=top_docs_html,
    category_html=category_html
)

# Write HTML file
with open('/Users/schneik/Documents/activity_analysis.html', 'w') as f:
    f.write(html_content)

# Also write a text summary
summary_text = f"""
ACTIVITY LOG ANALYSIS SUMMARY
{'=' * 80}

OVERVIEW:
Kevin Schneider's activity log shows a comprehensive record of CAD file management,
engineering design work, and 3D printing project documentation spanning from
{min_date.strftime('%B %d, %Y')} to {max_date.strftime('%B %d, %Y')}. The log encompasses 
{total_activities} total activities across {unique_dates} unique dates, with an average 
of {avg_per_day:.1f} activities per active day.

PRIMARY FOCUS AREAS:
The activity reveals heavy involvement in RC (radio control) vehicle design and
components, particularly focusing on:
- RC 1/10 scale 4WD Buggy projects (significant updates on Oct 23, 2025)
- RC 1/10 Egress vehicle development
- 3D printer maintenance and component documentation
- Mechanical libraries and parts standardization

TOP 10 MOST ACTIVE DOCUMENTS:
"""

for idx, (doc, count) in enumerate(top_docs, 1):
    summary_text += f"\n{idx}. {doc} - {count} activity(ies)"

summary_text += "\n\n\nACTIVITY BY CATEGORY:\n"
for cat, count in category_counter.most_common(10):
    summary_text += f"- {cat}: {count} activities\n"

summary_text += "\n\nACTIVITY BY DAY:\n"
for date, count in sorted_dates:
    summary_text += f"- {date}: {count} activities\n"

with open('/Users/schneik/Documents/activity_summary.txt', 'w') as f:
    f.write(summary_text)

print("Analysis complete!")
print(f"- Total activities: {total_activities}")
print(f"- Date range: {date_range}")
print(f"- Files created:")
print("  - activity_analysis.html (interactive visualization)")
print("  - activity_summary.txt (text summary)")
print("\nTop 5 documents:")
for doc, count in list(top_docs)[:5]:
    print(f"  - {doc}: {count} activities")
