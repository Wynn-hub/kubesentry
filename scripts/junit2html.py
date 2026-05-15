#!/usr/bin/env python3
"""Convert JUnit XML (from gotestsum) to a self-contained HTML report."""
import sys, xml.etree.ElementTree as ET, html, os
from datetime import datetime

def dur(s):
    try:
        t = float(s)
        return f"{t:.2f}s"
    except (TypeError, ValueError):
        return "—"

def badge(ok):
    return ('<span style="color:#22c55e;font-weight:700">PASS</span>' if ok
            else '<span style="color:#ef4444;font-weight:700">FAIL</span>')

def main():
    if len(sys.argv) < 3:
        print(f"usage: {sys.argv[0]} <input.xml> <output.html> [title]", file=sys.stderr)
        sys.exit(1)

    src, dst = sys.argv[1], sys.argv[2]
    title = sys.argv[3] if len(sys.argv) > 3 else os.path.basename(src)

    tree = ET.parse(src)
    root = tree.getroot()

    # Support both <testsuites> and bare <testsuite> roots
    suites = root.findall("testsuite") if root.tag == "testsuites" else [root]

    total = failures = errors = skipped = 0
    suite_rows = []

    for suite in suites:
        s_tests    = int(suite.get("tests",    0))
        s_fail     = int(suite.get("failures", 0))
        s_err      = int(suite.get("errors",   0))
        s_skip     = int(suite.get("skipped",  0))
        s_time     = suite.get("time", "")
        s_name     = suite.get("name", "(unnamed)")
        total    += s_tests
        failures += s_fail + s_err
        errors   += s_err
        skipped  += s_skip

        tc_rows = []
        for tc in suite.findall("testcase"):
            tc_name    = tc.get("name", "")
            tc_class   = tc.get("classname", "")
            tc_time    = tc.get("time", "")
            fail_el    = tc.find("failure")
            skip_el    = tc.find("skipped")
            err_el     = tc.find("error")

            if fail_el is not None or err_el is not None:
                status = '<span style="color:#ef4444;font-weight:700">FAIL</span>'
                detail_el = fail_el if fail_el is not None else err_el
                detail = f'<pre style="margin:4px 0 0;font-size:11px;white-space:pre-wrap;color:#fca5a5">{html.escape(detail_el.text or "")}</pre>'
            elif skip_el is not None:
                status = '<span style="color:#f59e0b;font-weight:700">SKIP</span>'
                detail = ""
            else:
                status = '<span style="color:#22c55e;font-weight:700">PASS</span>'
                detail = ""

            tc_rows.append(f"""
            <tr>
              <td style="padding:4px 8px;color:#94a3b8;font-size:12px">{html.escape(tc_class)}</td>
              <td style="padding:4px 8px;font-size:12px">{html.escape(tc_name)}{detail}</td>
              <td style="padding:4px 8px;text-align:right;font-size:12px;color:#94a3b8">{dur(tc_time)}</td>
              <td style="padding:4px 8px;text-align:center">{status}</td>
            </tr>""")

        ok = (s_fail + s_err) == 0
        suite_rows.append((ok, s_name, s_tests, s_fail, s_skip, dur(s_time), "".join(tc_rows)))

    passed = total - failures - skipped
    overall_ok = failures == 0
    generated = datetime.now().strftime("%Y-%m-%d %H:%M:%S")

    suite_html_parts = []
    for (ok, name, tests, fail, skip, time, tc_html) in suite_rows:
        bg = "#052e16" if ok else "#450a0a"
        border = "#166534" if ok else "#991b1b"
        suite_html_parts.append(f"""
        <details style="margin-bottom:12px;border:1px solid {border};border-radius:6px;overflow:hidden">
          <summary style="padding:10px 14px;background:{bg};cursor:pointer;list-style:none;display:flex;align-items:center;gap:10px">
            {badge(ok)}
            <span style="font-weight:600;flex:1">{html.escape(name)}</span>
            <span style="font-size:12px;color:#94a3b8">{tests} tests &nbsp;·&nbsp; {fail} failed &nbsp;·&nbsp; {skip} skipped &nbsp;·&nbsp; {time}</span>
          </summary>
          <table style="width:100%;border-collapse:collapse">
            <thead>
              <tr style="background:#1e293b">
                <th style="padding:6px 8px;text-align:left;font-size:11px;color:#94a3b8;width:30%">Package</th>
                <th style="padding:6px 8px;text-align:left;font-size:11px;color:#94a3b8">Test</th>
                <th style="padding:6px 8px;text-align:right;font-size:11px;color:#94a3b8;width:70px">Time</th>
                <th style="padding:6px 8px;text-align:center;font-size:11px;color:#94a3b8;width:60px">Status</th>
              </tr>
            </thead>
            <tbody>{tc_html}</tbody>
          </table>
        </details>""")

    overall_color = "#22c55e" if overall_ok else "#ef4444"
    overall_label = "ALL PASSED" if overall_ok else "FAILURES DETECTED"
    page = f"""<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>{html.escape(title)}</title>
  <style>
    *{{box-sizing:border-box;margin:0;padding:0}}
    body{{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#0f172a;color:#e2e8f0;padding:24px}}
    details>summary::-webkit-details-marker{{display:none}}
    tr:nth-child(even){{background:#0f172a}}
    tr:nth-child(odd){{background:#1e293b}}
  </style>
</head>
<body>
  <h1 style="font-size:20px;margin-bottom:4px">{html.escape(title)}</h1>
  <p style="font-size:12px;color:#64748b;margin-bottom:20px">Generated {generated}</p>
  <div style="display:flex;gap:16px;margin-bottom:24px;flex-wrap:wrap">
    <div style="padding:12px 20px;background:#1e293b;border-radius:8px;border-left:4px solid {overall_color}">
      <div style="font-size:12px;color:#94a3b8">Overall</div>
      <div style="font-size:18px;font-weight:700;color:{overall_color}">{overall_label}</div>
    </div>
    <div style="padding:12px 20px;background:#1e293b;border-radius:8px">
      <div style="font-size:12px;color:#94a3b8">Total</div>
      <div style="font-size:18px;font-weight:700">{total}</div>
    </div>
    <div style="padding:12px 20px;background:#1e293b;border-radius:8px">
      <div style="font-size:12px;color:#94a3b8">Passed</div>
      <div style="font-size:18px;font-weight:700;color:#22c55e">{passed}</div>
    </div>
    <div style="padding:12px 20px;background:#1e293b;border-radius:8px">
      <div style="font-size:12px;color:#94a3b8">Failed</div>
      <div style="font-size:18px;font-weight:700;color:#ef4444">{failures}</div>
    </div>
    <div style="padding:12px 20px;background:#1e293b;border-radius:8px">
      <div style="font-size:12px;color:#94a3b8">Skipped</div>
      <div style="font-size:18px;font-weight:700;color:#f59e0b">{skipped}</div>
    </div>
  </div>
  {"".join(suite_html_parts)}
</body>
</html>"""

    with open(dst, "w", encoding="utf-8") as f:
        f.write(page)
    print(f"[junit2html] {total} tests → {dst}")

if __name__ == "__main__":
    main()
