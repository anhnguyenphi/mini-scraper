#!/usr/bin/env python3
"""
crawl4ai_runner.py - Web scraper using the crawl4ai library.

(Must not be named crawl4ai.py — that shadows the crawl4ai package on sys.path.)

Usage:
    python crawl4ai_runner.py --url <url> [--markdown] [--timeout <seconds>]

Output:
    JSON with "html" and optionally "markdown" fields.
"""

import argparse
import asyncio
import json
import sys
from contextlib import redirect_stderr, redirect_stdout
from io import StringIO


def parse_args():
    parser = argparse.ArgumentParser(description="Scrape a webpage using crawl4ai")
    parser.add_argument("--url", required=True, help="URL to scrape")
    parser.add_argument("--markdown", action="store_true", help="Also return markdown")
    parser.add_argument("--timeout", type=int, default=60, help="Timeout in seconds")
    return parser.parse_args()


async def scrape(url: str, timeout: int, include_markdown: bool) -> dict:
    """Scrape a URL using crawl4ai."""
    try:
        from crawl4ai import AsyncWebCrawler, BrowserConfig, CrawlerRunConfig
    except ImportError:
        return {
            "error": "crawl4ai not installed. Run: pip install crawl4ai && crawl4ai-setup"
        }

    browser_config = BrowserConfig(
        headless=True,
        verbose=False,
    )

    try:
        run_config = CrawlerRunConfig(
            page_timeout=timeout * 1000,  # Convert to ms
            cache_mode="bypass",
            verbose=False,
            log_console=False,
        )
    except TypeError:
        run_config = CrawlerRunConfig(
            page_timeout=timeout * 1000,
            cache_mode="bypass",
        )

    # Crawl4AI prints [FETCH]/[SCRAPE] lines to stdout; keep stdout clean for JSON only.
    _noise_out, _noise_err = StringIO(), StringIO()
    try:
        with redirect_stdout(_noise_out), redirect_stderr(_noise_err):
            async with AsyncWebCrawler(config=browser_config) as crawler:
                result = await crawler.arun(url=url, config=run_config)

        if not result.success:
            return {"error": f"Crawl failed: {result.error_message}"}

        response = {"html": result.cleaned_html or ""}

        if include_markdown and result.markdown:
            response["markdown"] = result.markdown

        return response

    except Exception as e:
        return {"error": str(e)}


def main():
    args = parse_args()

    try:
        result = asyncio.run(scrape(args.url, args.timeout, args.markdown))
        print(json.dumps(result))
    except KeyboardInterrupt:
        print(json.dumps({"error": "Interrupted"}))
        sys.exit(1)
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.exit(1)


if __name__ == "__main__":
    main()
