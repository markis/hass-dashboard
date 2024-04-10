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
from dashboard.weather import Forecast, HourlyForecast, Weather, get_weather

API_URL: Final = os.getenv("HOMEASSISTANT_URL", "")
CALENDAR_ENTITY_IDS: Final = os.getenv("HOMEASSISTANT_CALENDARS", "").split(",")
OUTPUT_PATH: Final = os.getenv("OUTPUT_PATH", "output.png")
RENDER_HEIGHT: Final = int(os.getenv("RENDER_HEIGHT", "1200"))
RENDER_ROTATE: Final = int(os.getenv("RENDER_ROTATE", "270"))
RENDER_WIDTH: Final = int(os.getenv("RENDER_WIDTH", "820"))
TOKEN: Final = os.getenv("HOMEASSISTANT_TOKEN", "")
OPENWEATHER_API_KEY: Final = os.getenv("OPENWEATHERMAP_API_KEY", "")
LATITUDE: Final = float(os.getenv("LATITUDE", "0"))
LONGITUDE: Final = float(os.getenv("LONGITUDE", "0"))


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
    weather_task = get_weather(OPENWEATHER_API_KEY, LATITUDE, LONGITUDE)
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


def draw_weather_forecast(
    forecasts: list[HourlyForecast],
    width: int = 400,
    height: int = 80,
    padding: int = 20,
) -> str | None:
    """Draw a weather forecast as an SVG image."""
    import re

    import drawsvg as draw
    import numpy as np
    from scipy.interpolate import CubicSpline
    from scour import scour

    forecasts = forecasts[:24]

    # Initialize drawing with width and height
    d = draw.Drawing(width, height)

    # Get min and max temperatures for scaling
    min_temp = min(f.temp for f in forecasts)
    max_temp = max(f.temp for f in forecasts)

    # Get start and end times for scaling
    start_time = min(f.date for f in forecasts)
    end_time = max(f.date for f in forecasts)

    # Pre-compute scaling factors
    x_scale = (width - padding * 1.5) / (end_time - start_time).total_seconds()
    y_scale = (height - padding * 1.5) / (max_temp - min_temp)

    # Function to convert temperature and time to x and y coordinates
    def get_coords(temp: int, time: datetime) -> tuple[float, float]:
        x = padding * 1.5 + (time - start_time).total_seconds() * x_scale
        y = height - padding - (temp - min_temp) * y_scale
        return x, y

    # Prepare data for interpolation in one pass
    x_data, y_data = zip(*[get_coords(f.temp, f.date) for f in forecasts], strict=True)

    # Create cubic spline to smooth the line
    cs = CubicSpline(x_data, y_data)

    # Draw smooth line
    xnew = np.linspace(min(x_data), max(x_data), 100)
    ynew = cs(xnew)

    # Create a single polyline instead of multiple lines
    points = list(zip(xnew, ynew, strict=True))
    path = draw.Path(stroke_width=5, stroke="black", fill="none")
    path.M(points[0][0], points[0][1])
    for x, y in points[1:]:
        path.L(x, y)
    d.append(path)

    # Add x and y labels
    d.append(draw.Text(text=f"{int(min_temp)}°", font_size=20, x=0, y=height - 20))
    d.append(draw.Text(text=f"{int(max_temp)}°", font_size=20, x=0, y=20))

    # First, middle, and last time labels
    text = forecasts[0].date.strftime("%-I%p")
    d.append(draw.Text(text=text, font_size=20, x=30, y=height))
    text = forecasts[int(len(forecasts) / 2)].date.strftime("%-I%p")
    d.append(draw.Text(text=text, font_size=20, x=int((width - 20) / 2), y=height))
    text = forecasts[-1].date.strftime("%-I%p")
    d.append(draw.Text(text=text, font_size=20, x=width, y=height, text_anchor="end"))

    svg = d.as_svg()
    if svg is None:
        return None

    options = scour.sanitizeOptions()
    options.digits = 3
    options.cdigits = 3
    options.enable_viewboxing = False
    options.strip_xml_prolog = True
    options.indent_type = "none"

    # Return SVG source
    optimized_svg = scour.scourString(svg, options)
    return re.sub(r">\s+<", "><", optimized_svg)


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
    hourly_svg = draw_weather_forecast(weather.hourly)

    template = get_template(dashboard_template)
    rendered_html = template.render(
        weather=weather, dates_with_events=dates_with_events, events=events, hourly_svg=hourly_svg
    )
    css_str = get_file_contents(dashboard_css)
    Path("./output.html").write_text(
        f"""
        <link rel="stylesheet" href="./dashboard/static/style.css" />
        {rendered_html}
        """
    )

    hti = Html2Image(
        size=(RENDER_WIDTH, RENDER_HEIGHT),
        custom_flags=[
            "--lang=en",
            "--headless",
            "--hide-scrollbars",
            "--no-sandbox",
            "--no-first-run",
            "--disable-features=dbus",
            "--disable-sync",
            "--disable-gpu",
            "--disable-software-rasterizer",
            "--disable-dev-shm-usage",
            "--virtual-time-budget=10000",
            "--autoplay-policy=no-user-gesture-required",
            "--use-fake-ui-for-media-stream",
            "--use-fake-device-for-media-stream",
        ],
    )
    output_path = Path(OUTPUT_PATH)
    temp_output = output_path.with_suffix(f".tmp{output_path.suffix}")
    hti.screenshot(html_str=rendered_html, css_str=css_str, save_as=str(temp_output))

    with Image.open(temp_output) as im:
        if RENDER_ROTATE != 0:
            im.rotate(RENDER_ROTATE, expand=True).save(temp_output, optimize=True)
        else:
            im.save(temp_output, optimize=True)

    temp_output.rename(OUTPUT_PATH)
