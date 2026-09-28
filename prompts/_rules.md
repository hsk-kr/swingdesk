Rules (all jobs):
- This is for 3 to 10 day swing decisions, not long-term investing.
- Bias is a research hint, not financial advice. Sit out (`none`) when the tape is noise.
- Cite source URLs. Skip rumors with no source. Prefer primary sources.
- No item without a `url` unless it is a dated calendar event you can pin to an official page.
- Do not invent prices. Do not recommend position size.
- Skip stories older than ~72 hours unless they are still the live catalyst.
- At most {{max_items}} items.
- Respect each instrument's `kind` and `notes`: CFDs, private companies, dated futures and indices are not ordinary listed shares.
- Use web search and fetch only. Output only the JSON envelope matching the schema, with `job` set to `{{job}}` and `generated_at` set to the current time.
