You are deciding the standing translation of technical terms for a library of
computer science papers. The papers are being translated into {{LANGUAGE}},
and every one of them will use the renderings you give here. Choose the term
the field already uses. You are not inventing vocabulary, you are recording
it.

## What to answer

One line per term, in the order the terms are given, in this form:

    english term :: rendering

Nothing else on the line and nothing between the lines. No numbering, no
bullets, no commentary, no code fence around the block.

Where the term should stand in English, answer KEEP as the rendering:

    MapReduce :: KEEP

Where you are unsure which of two renderings the field prefers, add a third
field with the alternative and one short sentence saying why you chose the
first:

    throughput :: thông lượng :: also "băng thông", but that is bandwidth

Answer every term you are given, including the ones that are obvious. A term
you skip has to be asked for again.

## How to choose

Prefer the term a working practitioner in {{LANGUAGE}} would say to another
practitioner. Not the term in a dictionary, and not a literal translation of
the English parts.

Keep the English where the field keeps the English. Product names, algorithm
names, file formats and protocol names are not translated. Neither are terms
the field has simply borrowed, and pretending otherwise produces a word no
reader has ever seen. KEEP is a real answer and an honest one.

Render the whole term as one term. A term of several English words is one
concept and usually one phrase in {{LANGUAGE}}, not a word for word
substitution. "hash table" is a kind of table and "state machine" is not a
kind of machine.

Stay inside the domain the note gives. Where a term is marked as belonging to
one field, that is the sense to render, and the everyday sense of the same
English word is not what the papers mean. Where a note names two senses, give
the rendering that covers both if there is one, and otherwise the sense the
note puts first.

Be consistent with the terms already decided. Where two terms in the list
share a word, that word should come out the same way in both unless it means
something different.

## The terms

Each line is a term, and where there is a note it follows after a tab.

{{TERMS}}

## The language

{{RULES}}
