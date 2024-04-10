from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Final, cast

from pyowm import OWM
from pyowm.weatherapi25.one_call import OneCall
from pyowm.weatherapi25.one_call import Weather as OneCallWeather

from dashboard.cache import Cache
from dashboard.calendar import TZ

CACHE: "Cache[Weather]" = Cache(Path("weather.pickle"), 3600)
THUNDERSTORM: Final = 200
DRIZZLE: Final = 300
RAIN: Final = 500
SNOW: Final = 600


@dataclass(order=True)
class HourlyForecast:
    date: datetime

    temp: int
    condition: str
    weather_class: str

    @classmethod
    def from_one_call(cls, w: OneCallWeather) -> "HourlyForecast":
        weather_class, condition = _weather_to_icon_name(w.weather_code)
        return cls(
            condition=condition,
            weather_class=weather_class,
            date=datetime.fromtimestamp(w.ref_time, tz=TZ),
            temp=int(w.temp["temp"]),
        )


@dataclass(order=True)
class Forecast:
    date: datetime

    high_temp: int
    low_temp: int
    condition: str
    weather_class: str

    @classmethod
    def from_one_call(cls, w: OneCallWeather) -> "Forecast":
        weather_class, condition = _weather_to_icon_name(w.weather_code)
        return cls(
            condition=condition,
            weather_class=weather_class,
            date=datetime.fromtimestamp(w.ref_time, tz=TZ),
            high_temp=int(w.temp["max"]),
            low_temp=int(w.temp["min"]),
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
    def from_one_call(cls, one: OneCall) -> "Weather":
        weather_class, condition = _weather_to_icon_name(one.current.weather_code)
        temperature = int(one.current.temp["temp"])
        forecast_daily = cast(list[OneCallWeather], one.forecast_daily)
        forecasts = sorted(Forecast.from_one_call(forecast) for forecast in forecast_daily)
        forecast_hourly = cast(list[OneCallWeather], one.forecast_hourly)
        hourly = sorted(HourlyForecast.from_one_call(forecast) for forecast in forecast_hourly)
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
    data = CACHE.load_cache()
    if data is not None:
        return data

    owm = OWM(openweather_api_key)
    mgr = owm.weather_manager()
    one = mgr.one_call(lat=lat, lon=lon, exclude="minutely", units="imperial")
    data = Weather.from_one_call(one)
    if data is not None:
        CACHE.save_cache(data)
    return data


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
