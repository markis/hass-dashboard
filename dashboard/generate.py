import asyncio
import os
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from datetime import date, datetime, timedelta
from pathlib import Path
from typing import Final, cast

import aiohttp
from html2image import Html2Image
from jinja2 import Template
from PIL import Image

from dashboard.auth import BearerAuth
from dashboard.calendar import TZ, Event, get_calendars
from dashboard.weather import Weather, get_weather

API_URL: Final = os.getenv("HOMEASSISTANT_URL", "")
TOKEN: Final = os.getenv("HOMEASSISTANT_TOKEN", "")
OUTPUT_PATH: Final = os.getenv("OUTPUT_PATH", "output.png")
CALENDAR_ENTITY_IDS: Final = os.getenv("HOMEASSISTANT_CALENDARS", "").split(",")
WEATHER_ENTITY_ID: Final = os.getenv("HOMEASSISTANT_WEATHER_ENTITY_ID", "weather.openweathermap")
RENDER_WIDTH: Final = int(os.getenv("RENDER_WIDTH", "820"))
RENDER_HEIGHT: Final = int(os.getenv("RENDER_HEIGHT", "1200"))
RENDER_ROTATE: Final = int(os.getenv("RENDER_ROTATE", "270"))


@dataclass(order=True)
class DateCount:
    day: date
    events: int
    is_past: bool
    is_today: bool


def get_calendar_dates(current_date: datetime, weeks: int = 4) -> tuple[list[date], date]:
    """Generate a range of dates starting from the beginning of the current week.

    Args:
        current_date: The current date and time.
        weeks: Number of weeks to generate. Defaults to 4.

    Returns:
        A tuple containing a list of dates and the end date of the range.
    """
    start: datetime = current_date - timedelta(days=current_date.weekday())
    end: datetime = start + timedelta(weeks=weeks)
    return [(start + timedelta(days=i)).date() for i in range((end - start).days)], end.date()


def generate_dates_with_events(
    dates: list[date], events: Mapping[date, Sequence[Event]], today: date
) -> list[DateCount]:
    """Generate a list of DateCount objects based on the provided dates and events.

    Args:
        dates: List of dates to process.
        events: Mapping of dates to corresponding events.
        today: The current date.

    Returns:
        A list of DateCount objects representing each date with event information.
    """
    return [
        DateCount(
            day=dt,
            events=len(events.get(dt, [])),
            is_past=dt < today,
            is_today=dt == today,
        )
        for dt in dates
    ]


async def fetch_data(
    session: aiohttp.ClientSession,
    api_url: str,
    calendars: list[str],
    today: date,
    end: date,
) -> tuple[Weather, Mapping[date, Sequence[Event]]]:
    """Asynchronously fetch weather and calendar events.

    Args:
        session: The aiohttp client session.
        api_url: The API URL.
        token: The authentication token.
        calendars: List of calendar entity IDs.
        today: The current date.
        end: The end date of the calendar range.

    Returns:
        A tuple containing weather data and calendar events.
    """
    weather_task = get_weather(session, api_url, WEATHER_ENTITY_ID)
    events_task = get_calendars(session, api_url, calendars, today, end)
    task_results = await asyncio.gather(*[weather_task, events_task])
    weather = cast(Weather, task_results[0])
    events = cast(Mapping[date, Sequence[Event]], task_results[1])
    return weather, events


def get_template(template_path: Path) -> Template:
    """Read the data from the file and return it as a jinja2 template

    Args:
        template_path: The path to the template file.

    Returns:
        Template to render the html
    """
    with template_path.open() as template_file:
        template_html = template_file.read()
        return Template(template_html)


def get_file_contents(file_path: Path) -> str:
    """Get contents of file

    Args:
        output_path: The path of the file to read.

    Returns:
        File contents
    """
    with file_path.open() as f:
        return f.read()


async def generate_image() -> None:
    """Generate HTML content based on calendar and weather data and write it to an output file."""
    import dashboard

    dashboard_template = Path(dashboard.__file__).parent / "static" / "template.html"
    dashboard_css = Path(dashboard.__file__).parent / "static" / "style.css"

    current_datetime = datetime.now(tz=TZ)
    dates, end = get_calendar_dates(current_datetime)

    current_date = current_datetime.date()
    async with aiohttp.ClientSession(auth=BearerAuth(TOKEN)) as session:
        weather, events = await fetch_data(session, API_URL, CALENDAR_ENTITY_IDS, current_date, end)

    dates_with_events = generate_dates_with_events(dates, events, current_date)

    template = get_template(dashboard_template)
    rendered_html = template.render(
        weather=weather, dates_with_events=dates_with_events, events=events
    )
    css_str = get_file_contents(dashboard_css)

    hti = Html2Image(
        size=(RENDER_WIDTH, RENDER_HEIGHT),
        custom_flags=[
            "--hide-scrollbars",
            "--disable-dev-shm-usage",
            "--no-sandbox",
            "--lang=en",
            "--virtual-time-budget=10000",
        ],
    )
    hti.screenshot(html_str=rendered_html, css_str=css_str, save_as=OUTPUT_PATH)

    if RENDER_ROTATE != 0:
        with Image.open(OUTPUT_PATH) as im:
            im.rotate(RENDER_ROTATE, expand=True).save(OUTPUT_PATH)
