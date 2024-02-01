import json
from dataclasses import dataclass
from datetime import datetime
from typing import Any, Final

import Levenshtein
from aiohttp.client import ClientSession

WEATHER_ICON_NAMES: Final = {
    "day-sunny",
    "cloud",
    "cloudy",
    "cloudy-gusts",
    "cloudy-windy",
    "fog",
    "hail",
    "rain",
    "rain-mix",
    "rain-wind",
    "showers",
    "sleet",
    "snow",
    "sprinkle",
    "storm-showers",
    "thunderstorm",
    "snow-wind",
    "smog",
    "smoke",
    "lightning",
    "raindrops",
    "dust",
    "snowflake-cold",
    "windy",
    "strong-wind",
    "sandstorm",
    "earthquake",
    "fire",
    "flood",
    "meteor",
    "tsunami",
    "volcano",
    "hurricane",
    "tornado",
}


@dataclass(frozen=True, order=True)
class Forecast:
    date: datetime

    high_temp: int
    low_temp: int
    condition: str
    weather_class: str

    @staticmethod
    def get_datetime(date_str: str) -> datetime:
        return datetime.strptime(date_str, "%Y-%m-%dT%H:%M:%S%z")

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "Forecast":
        return cls(
            condition=d["condition"],
            weather_class=_weather_to_icon(d["condition"]),
            date=cls.get_datetime(d["datetime"]),
            high_temp=d["temperature"],
            low_temp=d["templow"],
        )


@dataclass(frozen=True)
class Weather:
    temperature: int
    forecasts: list[Forecast]

    high_temp: int
    low_temp: int
    condition: str
    weather_class: str

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "Weather":
        condition = d["state"]
        weather_class = _weather_to_icon(condition)
        temperature = d["attributes"]["temperature"]
        forecasts = sorted(Forecast.from_dict(forecast) for forecast in d["attributes"]["forecast"])
        todays_forecast = forecasts.pop(0)
        high_temp = todays_forecast.high_temp
        low_temp = todays_forecast.low_temp
        return cls(
            condition=condition,
            weather_class=weather_class,
            temperature=temperature,
            high_temp=high_temp,
            low_temp=low_temp,
            forecasts=forecasts,
        )


async def get_weather(session: ClientSession, api_url: str, weather_entity_id: str) -> Weather:
    url = f"{api_url}states/{weather_entity_id}"
    async with session.get(url, headers={"Content-Type": "application/json"}) as resp:
        data = await resp.text()
        result = json.loads(data)
    return Weather.from_dict(result)


def _weather_to_icon(weather_condition: str) -> str:
    result = "na"
    match weather_condition:
        case "clear-night":
            result = "stars"
        case "cloudy":
            result = "cloudy"
        case "exceptional":
            result = "fire"
        case "fog":
            result = "day-fog"
        case "hail":
            result = "hail"
        case "lightning-rainy" | "lightning":
            result = "storm-showers"
        case "partlycloudy":
            result = "day-cloudy"
        case "pouring":
            result = "rain"
        case "rainy":
            result = "showers"
        case "snowy-rainy":
            result = "rain-mix"
        case "snowy":
            result = "snowflake-cold"
        case "sunny":
            result = "day-sunny"
        case "windy-variant" | "windy":
            result = "strong-wind"
        case _:
            if weather_condition in WEATHER_ICON_NAMES:
                result = weather_condition
            else:
                min_distance = -1
                for candidate in WEATHER_ICON_NAMES:
                    distance = Levenshtein.distance(weather_condition, candidate)
                    if distance < min_distance:
                        min_distance = distance
                        result = candidate

    return f"wi wi-{result}"
