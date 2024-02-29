import asyncio
import json
import os
from collections import defaultdict
from collections.abc import Sequence
from dataclasses import dataclass
from datetime import date, datetime
from typing import Any, Final
from zoneinfo import ZoneInfo

from aiohttp.client import ClientSession

TZ: Final = ZoneInfo(os.getenv("TZ", "America/New_York"))


@dataclass(frozen=True, order=True)
class Event:
    start: datetime
    end: datetime
    name: str
    all_day: bool

    @staticmethod
    def get_datetime(name: str, d: dict[str, str]) -> tuple[datetime, bool]:
        if dt := d.get("dateTime"):
            return datetime.strptime(dt, "%Y-%m-%dT%H:%M:%S%z").astimezone(TZ), False
        if dt := d.get("date"):
            return datetime.strptime(dt, "%Y-%m-%d").astimezone(tz=TZ), True
        msg = f"The calendar event '{name}', is missing a start date"
        raise ValueError(msg)

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "Event":
        name = d["summary"]
        start, _ = cls.get_datetime(name, d["start"])
        end, all_day = cls.get_datetime(name, d["end"])
        return cls(start=start, end=end, name=name, all_day=all_day)


async def get_calendars(
    session: ClientSession,
    api_url: str,
    calendar_entity_ids: Sequence[str],
    start: date,
    end: date,
) -> dict[date, list[Event]]:
    start_str = datetime(start.year, start.month, start.day, tzinfo=TZ).isoformat()
    end_str = datetime(end.year, end.month, end.day, tzinfo=TZ).isoformat()

    async def get_calendar_data(calendar: str) -> Any:
        url = f"{api_url}calendars/calendar.{calendar}?start={start_str}&end={end_str}"
        async with session.get(url, headers={"Content-Type": "application/json"}) as resp:
            data = await resp.text()
            return json.loads(data)

    results = await asyncio.gather(
        *[get_calendar_data(calendar) for calendar in calendar_entity_ids]
    )

    grouped = defaultdict(list)
    for event in sorted({Event.from_dict(event) for result in results for event in result}):
        grouped[event.start.date()].append(event)

    return grouped
