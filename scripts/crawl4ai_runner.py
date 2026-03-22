#!/usr/bin/env python3
"""
crawl4ai_runner.py - Web scraper using the crawl4ai library.

(Must not be named crawl4ai.py — that shadows the crawl4ai package on sys.path.)

Usage (one-shot):
    python crawl4ai_runner.py --url <url> [--markdown] [--timeout <seconds>] ...

Persistent HTTP server (reuses one browser; requires: pip install aiohttp):
    python crawl4ai_runner.py serve --port 8765

Output (one-shot):
    JSON with "html" and optionally "markdown" fields.
"""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import sys
from contextlib import redirect_stderr, redirect_stdout
from dataclasses import dataclass
from io import StringIO
from typing import Any


@dataclass
class ScrapeParams:
    url: str
    timeout: int
    include_markdown: bool
    text_mode: bool
    light_mode: bool
    cache_mode: str
    wait_until: str
    delay_before_return_html: float
    extra_args: list[str]
    cdp_url: str
    browser_mode: str


def _cache_mode_value(mode: str):
    try:
        from crawl4ai import CacheMode

        m = mode.strip().lower()
        mapping = {
            "bypass": getattr(CacheMode, "BYPASS", None),
            "enabled": getattr(CacheMode, "ENABLED", None),
            "disabled": getattr(CacheMode, "DISABLED", None),
            "read_only": getattr(CacheMode, "READ_ONLY", None),
            "write_only": getattr(CacheMode, "WRITE_ONLY", None),
        }
        v = mapping.get(m)
        if v is not None:
            return v
    except Exception:
        pass
    return mode


def build_browser_config(
    text_mode: bool,
    light_mode: bool,
    extra_args: list[str],
    cdp_url: str,
    browser_mode: str,
):
    from crawl4ai import BrowserConfig

    kwargs: dict[str, Any] = {
        "headless": True,
        "verbose": False,
        "text_mode": text_mode,
        "light_mode": light_mode,
    }
    if extra_args:
        kwargs["extra_args"] = extra_args
    cu = (cdp_url or "").strip()
    if cu:
        kwargs["cdp_url"] = cu
        bm = (browser_mode or "custom").strip()
        kwargs["browser_mode"] = bm
    try:
        return BrowserConfig(**kwargs)
    except TypeError:
        kwargs.pop("text_mode", None)
        kwargs.pop("light_mode", None)
        return BrowserConfig(**kwargs)


def build_run_config(params: ScrapeParams):
    from crawl4ai import CrawlerRunConfig

    cm = _cache_mode_value(params.cache_mode)
    base: dict[str, Any] = {
        "page_timeout": params.timeout * 1000,
        "cache_mode": cm,
    }
    extended = {
        **base,
        "wait_until": params.wait_until,
        "delay_before_return_html": params.delay_before_return_html,
    }
    try:
        extended["verbose"] = False
        extended["log_console"] = False
        return CrawlerRunConfig(**extended)
    except TypeError:
        try:
            return CrawlerRunConfig(**base)
        except TypeError:
            return CrawlerRunConfig(page_timeout=params.timeout * 1000, cache_mode=cm)


async def scrape_once(params: ScrapeParams) -> dict:
    """Single scrape: create crawler, run, exit context (one-shot)."""
    try:
        from crawl4ai import AsyncWebCrawler
    except ImportError:
        return {
            "error": "crawl4ai not installed. Run: pip install crawl4ai && crawl4ai-setup"
        }

    browser_config = build_browser_config(
        params.text_mode,
        params.light_mode,
        params.extra_args,
        params.cdp_url,
        params.browser_mode,
    )
    run_config = build_run_config(params)

    _noise_out, _noise_err = StringIO(), StringIO()
    try:
        with redirect_stdout(_noise_out), redirect_stderr(_noise_err):
            async with AsyncWebCrawler(config=browser_config) as crawler:
                result = await crawler.arun(url=params.url, config=run_config)

        if not result.success:
            return {"error": f"Crawl failed: {result.error_message}"}

        response: dict[str, Any] = {"html": result.cleaned_html or ""}

        if params.include_markdown and result.markdown:
            response["markdown"] = result.markdown

        return response

    except Exception as e:
        return {"error": str(e)}


def params_from_namespace(ns: argparse.Namespace) -> ScrapeParams:
    return ScrapeParams(
        url=ns.url,
        timeout=ns.timeout,
        include_markdown=ns.markdown,
        text_mode=ns.text_mode,
        light_mode=ns.light_mode,
        cache_mode=ns.cache_mode,
        wait_until=ns.wait_until,
        delay_before_return_html=ns.delay_before_return_html,
        extra_args=list(ns.extra_arg or []),
        cdp_url=getattr(ns, "cdp_url", "") or "",
        browser_mode=getattr(ns, "browser_mode", "") or "",
    )


def add_scrape_arguments(p: argparse.ArgumentParser, *, url_required: bool = True) -> None:
    p.add_argument("--url", required=url_required, default="", help="URL to scrape")
    p.add_argument("--markdown", action="store_true", help="Also return markdown")
    p.add_argument("--timeout", type=int, default=60, help="Timeout in seconds")
    p.add_argument(
        "--text-mode",
        action=argparse.BooleanOptionalAction,
        default=True,
        help="Browser text mode (faster; fewer images). Default: true",
    )
    p.add_argument(
        "--light-mode",
        action=argparse.BooleanOptionalAction,
        default=True,
        help="Light browser mode (faster). Default: true",
    )
    p.add_argument(
        "--cache-mode",
        default="bypass",
        help="Cache: bypass|enabled|disabled|read_only|write_only (default: bypass)",
    )
    p.add_argument(
        "--wait-until",
        default="domcontentloaded",
        help="Playwright wait_until (default: domcontentloaded)",
    )
    p.add_argument(
        "--delay-before-return-html",
        type=float,
        default=0.0,
        help="Extra delay before capture in seconds (default: 0)",
    )
    p.add_argument(
        "--extra-arg",
        action="append",
        dest="extra_arg",
        metavar="FLAG",
        help="Extra Chromium flag (repeatable), e.g. --extra-arg --disable-gpu",
    )
    p.add_argument(
        "--cdp-url",
        default="",
        help="WebSocket CDP URL to attach (e.g. ws://127.0.0.1:9222). Empty = launch browser",
    )
    p.add_argument(
        "--browser-mode",
        default="custom",
        help="When using --cdp-url: browser_mode for crawl4ai (default: custom)",
    )


def parse_args(argv: list[str] | None) -> tuple[str, argparse.Namespace]:
    if argv is None:
        argv = sys.argv[1:]

    if argv and argv[0] == "serve":
        sp = argparse.ArgumentParser(
            prog="crawl4ai_runner.py serve",
            description="Persistent crawl4ai HTTP server (pip install aiohttp)",
        )
        sp.add_argument("--host", default="127.0.0.1", help="Bind address")
        sp.add_argument("--port", type=int, default=8765, help="Port")
        sp.add_argument(
            "--auth-token",
            default="",
            help="If set, require X-API-Key header (or env CRAWL4AI_SERVE_TOKEN)",
        )
        add_scrape_arguments(sp, url_required=False)
        ns = sp.parse_args(argv[1:])
        return "serve", ns

    scrape = argparse.ArgumentParser(
        description="Scrape one URL with crawl4ai and print JSON",
    )
    add_scrape_arguments(scrape)
    ns = scrape.parse_args(argv)
    return "scrape", ns


async def _run_serve(args: argparse.Namespace) -> None:
    try:
        from aiohttp import web
    except ImportError:
        print(
            json.dumps({"error": "serve requires aiohttp: pip install aiohttp"}),
            file=sys.stderr,
        )
        sys.exit(1)

    try:
        from crawl4ai import AsyncWebCrawler
    except ImportError:
        print(
            json.dumps({"error": "crawl4ai not installed. Run: pip install crawl4ai && crawl4ai-setup"}),
            file=sys.stderr,
        )
        sys.exit(1)

    auth = (args.auth_token or os.environ.get("CRAWL4AI_SERVE_TOKEN", "")).strip()

    browser_config = build_browser_config(
        args.text_mode,
        args.light_mode,
        list(args.extra_arg or []),
        getattr(args, "cdp_url", "") or "",
        getattr(args, "browser_mode", "") or "",
    )

    crawler_holder: dict[str, Any] = {}
    lock = asyncio.Lock()

    async def startup(_app: web.Application) -> None:
        crawler = AsyncWebCrawler(config=browser_config)
        await crawler.start()
        crawler_holder["c"] = crawler

    async def cleanup(_app: web.Application) -> None:
        c = crawler_holder.get("c")
        if c is not None:
            await c.close()

    async def health(_request: web.Request) -> web.Response:
        return web.json_response({"status": "ok"})

    async def scrape_req(request: web.Request) -> web.Response:
        if auth:
            if request.headers.get("X-API-Key", "").strip() != auth:
                return web.json_response({"error": "unauthorized"}, status=401)

        try:
            body = await request.json()
        except json.JSONDecodeError:
            return web.json_response({"error": "invalid JSON"}, status=400)

        url = (body.get("url") or "").strip()
        if not url:
            return web.json_response({"error": "url is required"}, status=400)

        p = ScrapeParams(
            url=url,
            timeout=int(body.get("timeout", 60)),
            include_markdown=bool(body.get("markdown", False)),
            text_mode=bool(body.get("text_mode", args.text_mode)),
            light_mode=bool(body.get("light_mode", args.light_mode)),
            cache_mode=str(body.get("cache_mode", args.cache_mode)),
            wait_until=str(body.get("wait_until", args.wait_until)),
            delay_before_return_html=float(body.get("delay_before_return_html", args.delay_before_return_html)),
            extra_args=list(body.get("extra_args") or args.extra_arg or []),
            cdp_url=str(body.get("cdp_url") or getattr(args, "cdp_url", "") or ""),
            browser_mode=str(body.get("browser_mode") or getattr(args, "browser_mode", "") or ""),
        )
        run_config = build_run_config(p)

        async with lock:
            result = await crawler_holder["c"].arun(url=p.url, config=run_config)

        if not result.success:
            return web.json_response({"error": f"Crawl failed: {result.error_message}"}, status=502)

        out: dict[str, Any] = {"html": result.cleaned_html or ""}
        if p.include_markdown and result.markdown:
            out["markdown"] = result.markdown
        return web.json_response(out)

    app = web.Application()
    app.router.add_get("/health", health)
    app.router.add_post("/scrape", scrape_req)
    app.on_startup.append(startup)
    app.on_cleanup.append(cleanup)

    runner = web.AppRunner(app)
    await runner.setup()
    site = web.TCPSite(runner, args.host, args.port)
    await site.start()
    print(
        json.dumps(
            {
                "listening": f"http://{args.host}:{args.port}",
                "endpoints": ["/health", "POST /scrape"],
            }
        ),
        flush=True,
    )
    await asyncio.Event().wait()


def main() -> None:
    argv = sys.argv[1:]
    try:
        command, ns = parse_args(argv)
    except SystemExit:
        raise
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.exit(1)

    if command == "serve":
        try:
            asyncio.run(_run_serve(ns))
        except KeyboardInterrupt:
            print(json.dumps({"error": "Interrupted"}))
            sys.exit(1)
        return

    params = params_from_namespace(ns)
    try:
        result = asyncio.run(scrape_once(params))
        print(json.dumps(result))
    except KeyboardInterrupt:
        print(json.dumps({"error": "Interrupted"}))
        sys.exit(1)
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.exit(1)


if __name__ == "__main__":
    main()
