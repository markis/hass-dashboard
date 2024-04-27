from dataclasses import dataclass
from datetime import datetime
from http import HTTPStatus
from typing import Final, Self

import aiohttp

from dashboard.cache import Cache
from dashboard.calendar import TZ
from dashboard.typed_def import (
    OneCallDailyWeather,
    OneCallHourlyWeather,
    OneCallWeatherData,
)

THUNDERSTORM: Final = 200
DRIZZLE: Final = 300
RAIN: Final = 500
SNOW: Final = 600
weather_cache: "Cache[Weather]" = Cache(600)


@dataclass(order=True)
class HourlyForecast:
    date: datetime

    temp: int
    condition: str
    weather_class: str

    @classmethod
    def from_one_call(cls, w: OneCallHourlyWeather) -> Self:
        weather_id = next(iter(w["weather"]), {"id": 0})["id"]
        weather_class, condition = _weather_to_icon_name(weather_id)
        return cls(
            condition=condition,
            weather_class=weather_class,
            date=datetime.fromtimestamp(w["dt"], tz=TZ),
            temp=int(w["temp"]),
        )


@dataclass(order=True)
class Forecast:
    date: datetime

    high_temp: int
    low_temp: int
    condition: str
    weather_class: str

    @classmethod
    def from_one_call(cls, w: OneCallDailyWeather) -> Self:
        weather_id = next(iter(w["weather"]), {"id": 0})["id"]
        weather_class, condition = _weather_to_icon_name(weather_id)
        return cls(
            condition=condition,
            weather_class=weather_class,
            date=datetime.fromtimestamp(w["dt"], tz=TZ),
            high_temp=int(w["temp"]["max"]),
            low_temp=int(w["temp"]["min"]),
        )


@dataclass()
class Weather:
    temperature: int
    forecasts: list[Forecast]
    hourly: list[HourlyForecast]

    high_temp: int
    low_temp: int
    condition: str
    weather_class: str

    @classmethod
    def from_one_call(cls, one: OneCallWeatherData) -> Self:
        weather_id = next(iter(one["current"]["weather"]), {"id": 0})["id"]
        weather_class, condition = _weather_to_icon_name(weather_id)
        temperature = int(one["current"]["temp"])
        forecasts = sorted(Forecast.from_one_call(forecast) for forecast in one["daily"])
        hourly = sorted(HourlyForecast.from_one_call(forecast) for forecast in one["hourly"])
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
            hourly=hourly,
        )


async def get_weather(openweather_api_key: str, lat: float, lon: float) -> Weather:
    """
    Fetch weather data using OpenWeatherMap's One Call API 3.0 asynchronously.

    Args:
        openweather_api_key: Your OpenWeatherMap API key
        lat: Latitude of the location
        lon: Longitude of the location

    Returns:
        Weather data class
    """
    cache_key = f"{lat},{lon}"
    weather, stale = weather_cache.load(cache_key)
    if weather is not None and not stale:
        return weather

    params = {
        "lat": lat,
        "lon": lon,
        "appid": openweather_api_key,
        "units": "imperial",  # Use 'metric' for Celsius
        "exclude": "minutely",  # Exclude minute-by-minute data
    }
    url = "https://api.openweathermap.org/data/3.0/onecall"

    async with aiohttp.ClientSession() as session, session.get(url, params=params) as response:
        if response.status == HTTPStatus.OK:
            data = await response.json()
            weather = Weather.from_one_call(data)
            weather_cache.save(cache_key, weather)

    assert weather is not None
    return weather


def _weather_to_icon_name(weather_code: int) -> tuple[str, str]:
    """
    Convert a weather code to a Weather Icons icon name.

    From https://openweathermap.org/weather-conditions
    CSS class names are from https://erikflowers.github.io/weather-icons/
    """
    css_class = "na"
    name = "Unknown"

    match weather_code:
        case _ if THUNDERSTORM <= weather_code <= THUNDERSTORM + 99:
            css_class = "thunderstorm"
            name = "Thunderstorm"
        case _ if DRIZZLE <= weather_code <= DRIZZLE + 99:
            css_class = "sprinkle"
            name = "Drizzle"
        case _ if RAIN <= weather_code <= RAIN + 99:
            css_class = "rain"
            name = "Rain"
        case _ if SNOW <= weather_code <= SNOW + 99:
            css_class = "snow"
            name = "Snow"
        case 701:
            css_class = "fog"
            name = "Mist"
        case 711:
            css_class = "smoke"
            name = "Smoke"
        case 721:
            css_class = "day-haze"
            name = "Haze"
        case 731 | 761:
            css_class = "dust"
            name = "Dust"
        case 741:
            css_class = "fog"
            name = "Fog"
        case 751:
            css_class = "sandstorm"
            name = "Sand"
        case 762:
            css_class = "volcano"
            name = "Ash"
        case 771:
            css_class = "strong-wind"
            name = "Squall"
        case 781:
            css_class = "tornado"
            name = "Tornado"
        case 800:
            css_class = "day-sunny"
            name = "Clear"
        case 801:
            css_class = "cloud"
            name = "Few Clouds"
        case 802 | 803:
            css_class = "cloudy"
            name = "Partly Cloudy"
        case 804:
            css_class = "cloudy"
            name = "Overcast"

    return f"wi wi-{css_class}", name
