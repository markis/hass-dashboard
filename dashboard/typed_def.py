from typing import TypedDict


class OneCallWeather(TypedDict):
    id: int
    main: str
    description: str
    icon: str


class OneCallTemp(TypedDict):
    day: float
    min: float
    max: float
    night: float
    eve: float
    morn: float


class OneCallFeelsLike(TypedDict):
    day: float
    night: float
    eve: float
    morn: float


class OneCallRain(TypedDict):
    h1: float


class OneCallCurrentWeather(TypedDict):
    dt: int
    sunrise: int
    sunset: int
    temp: float
    feels_like: float
    pressure: int
    humidity: int
    dew_point: float
    uvi: float
    clouds: int
    visibility: int
    wind_speed: float
    wind_deg: int
    weather: list[OneCallWeather]


class OneCallHourlyWeather(TypedDict):
    dt: int
    temp: float
    feels_like: float
    pressure: int
    humidity: int
    dew_point: float
    uvi: float
    clouds: int
    visibility: int
    wind_speed: float
    wind_deg: int
    wind_gust: float
    weather: list[OneCallWeather]
    pop: float
    rain: OneCallRain | None


class OneCallDailyWeather(TypedDict):
    dt: int
    sunrise: int
    sunset: int
    moonrise: int
    moonset: int
    moon_phase: float
    temp: OneCallTemp
    feels_like: OneCallFeelsLike
    pressure: int
    humidity: int
    dew_point: float
    wind_speed: float
    wind_deg: int
    wind_gust: float
    weather: list[OneCallWeather]
    clouds: int
    pop: float
    rain: float
    uvi: float


class OneCallWeatherData(TypedDict):
    lat: float
    lon: float
    timezone: str
    timezone_offset: int
    current: OneCallCurrentWeather
    hourly: list[OneCallHourlyWeather]
    daily: list[OneCallDailyWeather]
