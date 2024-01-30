import json
from dataclasses import dataclass
from datetime import datetime
from typing import Any

from aiohttp.client import ClientSession


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
            weather_class=_weather_to_font_awesome(d["condition"]),
            date=cls.get_datetime(d["datetime"]),
            high_temp=d["temperature"],
            low_temp=d["templow"],
        )


@dataclass(frozen=True)
class Weather:
    condition: str
    weather_class: str
    temperature: int
    high_temp: int
    low_temp: int
    forecasts: list[Forecast]

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "Weather":
        condition = d["state"]
        temperature = d["attributes"]["temperature"]
        forecasts = sorted(Forecast.from_dict(forecast) for forecast in d["attributes"]["forecast"])
        todays_forecast = forecasts.pop(0)
        weather_class = todays_forecast.weather_class
        high_temp = todays_forecast.high_temp
        low_temp = todays_forecast.low_temp
        return cls(condition, weather_class, temperature, high_temp, low_temp, forecasts)


async def get_weather(session: ClientSession, api_url: str, weather_entity_id: str) -> Weather:
    url = f"{api_url}states/{weather_entity_id}"
    async with session.get(url, headers={"Content-Type": "application/json"}) as resp:
        data = await resp.text()
        result = json.loads(data)
    return Weather.from_dict(result)


def _weather_to_font_awesome(weather_condition: str) -> str:
    result = "fa-circle-question"
    match weather_condition:
        case "clear" | "sun" | "sunny":
            result = "fa-sun"
        case "cloudy":
            result = "fa-cloud"
        case "partlycloudy":
            result = "fa-cloud-sun"
        case "rain" | "rainy":
            result = "fa-cloud-rain"
        case "showers":
            result = "fa-cloud-rain"
        case "thunder" | "thunderstorm":
            result = "fa-cloud-bolt"
    return result
