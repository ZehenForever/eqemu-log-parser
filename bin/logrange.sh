#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  logrange.sh -f LOGFILE -s "Sat Jan 24 23:15:58 2026" -e "Sat Jan 24 23:30:06 2026" [-p SECONDS]

Options:
  -f  Path to log file
  -s  Start timestamp (exact format: "Dow Mon DD HH:MM:SS YYYY")
  -e  End timestamp   (exact format: "Dow Mon DD HH:MM:SS YYYY")
  -p  Padding seconds before/after (default: 120)

Examples:
  ./logrange.sh -f server.log \
    -s "Sat Jan 24 23:15:58 2026" \
    -e "Sun Jan 25 00:30:06 2026" \
    -p 120

Notes:
  - Handles ranges that cross midnight (and any multi-day range).
  - Prints lines whose effective timestamp falls within [start-pad, end+pad].
  - Lines without a leading timestamp inherit the previous timestamp (useful for multiline blocks).
EOF
}

file=""
start=""
end=""
pad="120"

while getopts ":f:s:e:p:h" opt; do
  case "$opt" in
    f) file="$OPTARG" ;;
    s) start="$OPTARG" ;;
    e) end="$OPTARG" ;;
    p) pad="$OPTARG" ;;
    h) usage; exit 0 ;;
    \?) echo "Unknown option: -$OPTARG" >&2; usage; exit 2 ;;
    :)  echo "Missing argument for -$OPTARG" >&2; usage; exit 2 ;;
  esac
done

if [[ -z "${file}" || -z "${start}" || -z "${end}" ]]; then
  echo "Error: -f, -s, and -e are required." >&2
  usage
  exit 2
fi

if [[ ! -f "${file}" ]]; then
  echo "Error: file not found: ${file}" >&2
  exit 2
fi

# Basic validation: pad must be integer >= 0
if ! [[ "${pad}" =~ ^[0-9]+$ ]]; then
  echo "Error: padding (-p) must be a non-negative integer (seconds)." >&2
  exit 2
fi

gawk -v start="${start}" -v end="${end}" -v pad="${pad}" '
function month_num(m,  a) {
  split("Jan Feb Mar Apr May Jun Jul Aug Sep Oct Nov Dec", a, " ")
  for (i=1; i<=12; i++) if (a[i]==m) return i
  return 0
}

# Convert civil date to days since 1970-01-01 (UTC-free; we treat input as local clock time consistently)
# Algorithm: Howard Hinnant (public domain style), used widely.
function days_from_civil(y, m, d,  era, yoe, doy, doe) {
  y -= (m <= 2)
  era = int((y >= 0 ? y : y-399) / 400)
  yoe = y - era * 400
  doy = int((153*(m + (m > 2 ? -3 : 9)) + 2)/5) + d - 1
  doe = yoe*365 + int(yoe/4) - int(yoe/100) + doy
  return era*146097 + doe - 719468
}

# Parse: "Sat Jan 24 23:15:58 2026"
function parse_user_ts(ts,  parts, n, mon, day, hms, yr, h, mi, s, m) {
  n = split(ts, parts, " ")
  if (n != 5) return -1

  mon = parts[2]
  day = parts[3]
  hms = parts[4]
  yr  = parts[5]

  m = month_num(mon)
  if (m == 0) return -1

  split(hms, t, ":")
  if (length(t[1])==0 || length(t[2])==0 || length(t[3])==0) return -1

  h  = t[1] + 0
  mi = t[2] + 0
  s  = t[3] + 0

  return (days_from_civil(yr+0, m, day+0) * 86400) + (h*3600) + (mi*60) + s
}

BEGIN {
  start_epoch = parse_user_ts(start)
  end_epoch   = parse_user_ts(end)

  if (start_epoch < 0) {
    print "Error: could not parse -s timestamp. Expected: \"Dow Mon DD HH:MM:SS YYYY\"" > "/dev/stderr"
    exit 2
  }
  if (end_epoch < 0) {
    print "Error: could not parse -e timestamp. Expected: \"Dow Mon DD HH:MM:SS YYYY\"" > "/dev/stderr"
    exit 2
  }

  # Apply padding
  start_epoch -= pad
  end_epoch   += pad

  # Guard: if user swapped start/end by mistake
  if (end_epoch < start_epoch) {
    tmp = start_epoch; start_epoch = end_epoch; end_epoch = tmp
  }

  have_epoch = 0
}

{
  # Match leading timestamp like: [Sat Jan 24 23:15:58 2026]
  if (match($0, /^\[[A-Za-z]{3} ([A-Za-z]{3}) ([0-9]{1,2}) ([0-9]{2}:[0-9]{2}:[0-9]{2}) ([0-9]{4})\]/, m)) {
    mon = m[1]
    day = m[2] + 0
    hms = m[3]
    yr  = m[4] + 0

    mm = month_num(mon)
    split(hms, t, ":"); h=t[1]+0; mi=t[2]+0; s=t[3]+0
    epoch = (days_from_civil(yr, mm, day) * 86400) + (h*3600) + (mi*60) + s
    have_epoch = 1
  }

  # If this is a continuation line before any timestamped line, ignore it.
  if (!have_epoch) next

  if (epoch >= start_epoch && epoch <= end_epoch) print
}
' "${file}"
