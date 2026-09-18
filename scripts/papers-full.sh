#!/bin/bash
# Read every paper in the corpus in full and push it, one paper at a time.
#
# This is the autonomous pipeline. It ran for weeks on a host with nobody
# watching it and it lived in that host's home directory, which meant the one
# piece of the system that decides what order the work happens in was the one
# piece nobody could read, review or restore. It lives here now.
#
# A paper is taken all the way to the end before the next one starts: read,
# mapped, cropped, cited, split, tagged, translated and then published, the
# English and the translations together in one pull request.
#
# Translation comes before publication and not after it. The publish step is a
# gate as well as a push: it runs the hard audit rules and refuses a paper any
# of them names. Half a translation is one of the things those rules refuse, so
# gating the translation on the gate is a deadlock, and a run sat on the
# Razborov paper doing exactly that: five of its nine Vietnamese sections were
# written, rule T04 saw the hole in the numbering, the paper was held, and the
# translation that would have filled the hole was the thing being held back.
#
# A paper the gate refuses stays in the working tree with everything written
# for it, and a later pass over the list picks it up. There are several passes
# because most of what holds a paper back is temporary: a model that would not
# answer, an account that had spent its uploads for the day, a section that came
# back as an apology. A pass over a finished paper is nearly free, because every
# step of this leaves work that is already done alone, so the cost of the later
# passes is the papers that still need something.
#
# The passes are bounded. A paper that fails the same way five times in a row is
# a paper that needs a person to look at it, and a loop with no end on it would
# spend the day asking a model the same question.
#
# usage: papers-full.sh [list] [passes]
#
# list is a file of paper identifiers, one per line, and defaults to every
# paper in the manifest in manifest order. passes defaults to 5.
#
# It reads four things from the environment:
#
#   PAPERS_CORPUS   a checkout of tamnd/papers, else the working directory
#   PAPERS_LANGS    the languages to translate into, comma separated, default vi
#   PAPERS_ENV      a file to source first, for the keys the routes name
#   GH_TOKEN        a token that may open and merge a pull request
#
# Nothing here names a host. The hosts are in the routing table the papers
# command reads, which is not in any repository and is not meant to be.
set -u

if [ -n "${PAPERS_ENV:-}" ]; then
	set -a
	# shellcheck disable=SC1090
	. "$PAPERS_ENV"
	set +a
fi
if [ -n "${PAPERS_CORPUS:-}" ]; then
	cd "$PAPERS_CORPUS" || exit 1
fi
langs=${PAPERS_LANGS:-vi}
passes=${2:-5}

stamp() { date -u +%Y%m%dT%H%M%SZ; }

# The list is read once and held, because a run is a day and a half long and a
# list that changed under it would be a run that skipped a paper it had never
# looked at.
list=${1:-}
if [ -n "$list" ]; then
	ids=$(grep -v '^[[:space:]]*$' "$list")
else
	ids=$(papers list -ids) || exit 1
fi
total=$(printf '%s\n' "$ids" | grep -c .)

for pass in $(seq 1 "$passes"); do
	echo "=== pass $pass of $passes over $total papers, $(stamp)"
	n=0
	held=0
	while read -r id; do
		[ -z "$id" ] && continue
		n=$((n + 1))
		echo "=== $id, $n of $total, pass $pass, $(stamp)"
		# Ahead of the work rather than after it, so that a run that has been
		# going for a day is writing against what the corpus holds now and its
		# pull request is a fast forward.
		git fetch -q origin && git merge -q --ff-only origin/main

		papers classify -id "$id" -again 2>&1
		papers extract -id "$id" 2>&1
		papers extract -id "$id" -path vision 2>&1
		papers pagemap -id "$id" 2>&1
		papers figures -id "$id" 2>&1
		papers refs build -id "$id" 2>&1
		# -prune because a section the English lost is a translation with
		# nothing above it, which rules T04, T07 and G04 all refuse and which
		# no later step will ever clean up on its own.
		papers split -id "$id" -prune 2>&1
		papers tags assign -id "$id" 2>&1
		for lang in ${langs//,/ }; do
			papers translate -id "$id" -lang "$lang" 2>&1
		done
		papers report coverage -write -quiet 2>&1
		note="This is $id read in full, which the corpus now publishes whatever the licence says."
		if ! papers publish -id "$id" -note "$note" 2>&1; then
			echo "=== $id is held back by the audit and stays in the working tree, $(stamp)"
			held=$((held + 1))
		fi
		echo "=== $id done, $(stamp)"
		# The hosts behind the routing table are shared and rate limited, and a
		# run that comes straight back for the next paper spends its allowance
		# faster than it earns anything with it.
		sleep 20
	done <<-EOF
		$ids
	EOF
	echo "=== pass $pass is done, papers held back: $held, $(stamp)"
	if [ "$held" -eq 0 ]; then
		break
	fi
done
echo "=== every paper in the list is done, $(stamp)"
