CREATE TABLE IF NOT EXISTS allowed_domains (
   id SERIAL PRIMARY KEY,
   domain VARCHAR(255) NOT NULL UNIQUE,
   description VARCHAR(255),
   is_active BOOLEAN DEFAULT TRUE,
   created_at TIMESTAMP DEFAULT NOW()
);

-- Разрешенный список ресурсов
INSERT INTO allowed_domains (domain, description) VALUES
  -- Маркетплейсы
  ('ozon.ru', 'Ozon'),
  ('ozon.com', 'Ozon'),
  ('wildberries.ru', 'Wildberries'),
  ('wb.ru', 'Wildberries'),
  ('aliexpress.ru', 'AliExpress'),
  ('aliexpress.com', 'AliExpress'),
  ('market.yandex.ru', 'Яндекс.Маркет'),
  ('ya.ru', 'Яндекс'),
  ('dns-shop.ru', 'DNS'),
  ('mvideo.ru', 'М.Видео'),
  ('eldorado.ru', 'Эльдорадо'),
  ('citilink.ru', 'Ситилинк'),
  ('sbermegamarket.ru', 'СберМегаМаркет'),

  -- Видеохостинги
  ('youtube.com', 'YouTube'),
  ('youtu.be', 'YouTube'),
  ('rutube.ru', 'Rutube'),

  -- Госуслуги и порталы
  ('gosuslugi.ru', 'Госуслуги'),
  ('gu.spb.ru', 'Госуслуги Санкт-Петербурга'),
  ('mos.ru', 'Портал мэра Москвы'),
  ('government.ru', 'Правительство РФ'),
  ('kremlin.ru', 'Президент РФ'),
  ('duma.gov.ru', 'Государственная Дума'),
  ('council.gov.ru', 'Совет Федерации'),

  -- Налоги и финансы
  ('nalog.gov.ru', 'ФНС России'),
  ('nalog.ru', 'ФНС России'),
  ('cbr.ru', 'Центральный банк РФ'),
  ('minfin.gov.ru', 'Минфин России'),
  ('roskazna.gov.ru', 'Федеральное казначейство'),
  ('pfr.gov.ru', 'Социальный фонд России (бывш. ПФР)'),
  ('sfr.gov.ru', 'Социальный фонд России'),
  ('fss.ru', 'Фонд социального страхования'),

  -- Правоохранительные органы и безопасность
  ('mvd.ru', 'МВД России'),
  ('мвд.рф', 'МВД России'),
  ('fsb.ru', 'ФСБ России'),
  ('mchs.gov.ru', 'МЧС России'),
  ('fsin.gov.ru', 'ФСИН России'),
  ('fssp.gov.ru', 'ФССП России (судебные приставы)'),
  ('sledcom.ru', 'Следственный комитет'),
  ('genproc.gov.ru', 'Генеральная прокуратура'),
  ('rkn.gov.ru', 'Роскомнадзор'),

  -- Суды и правовые системы
  ('sudrf.ru', 'Суды РФ'),
  ('vsrf.ru', 'Верховный Суд РФ'),
  ('ksrf.ru', 'Конституционный Суд РФ'),
  ('arbitr.ru', 'Арбитражные суды'),
  ('pravo.gov.ru', 'Официальный интернет-портал правовой информации'),

  -- Министерства
  ('mid.ru', 'МИД России'),
  ('mil.ru', 'Минобороны России'),
  ('minzdrav.gov.ru', 'Минздрав России'),
  ('edu.gov.ru', 'Минпросвещения России'),
  ('minobrnauki.gov.ru', 'Минобрнауки России'),
  ('mintrud.gov.ru', 'Минтруд России'),
  ('minstroyrf.gov.ru', 'Минстрой России'),
  ('mintrans.gov.ru', 'Минтранс России'),
  ('minpromtorg.gov.ru', 'Минпромторг России'),
  ('minjust.gov.ru', 'Минюст России'),
  ('mnr.gov.ru', 'Минприроды России'),
  ('mcx.gov.ru', 'Минсельхоз России'),
  ('digital.gov.ru', 'Минцифры России'),
  ('economy.gov.ru', 'Минэкономразвития России'),
  ('energo.gov.ru', 'Минэнерго России'),

  -- Федеральные службы и агентства
  ('rosreestr.gov.ru', 'Росреестр'),
  ('rospotrebnadzor.ru', 'Роспотребнадзор'),
  ('rostrud.gov.ru', 'Роструд'),
  ('rosstat.gov.ru', 'Росстат'),
  ('customs.gov.ru', 'ФТС России (таможня)'),
  ('fas.gov.ru', 'ФАС России (антимонопольная служба)'),
  ('fsa.gov.ru', 'Росаккредитация'),
  ('favt.gov.ru', 'Росавиация'),
  ('gibdd.ru', 'ГИБДД'),
  ('гибдд.рф', 'ГИБДД'),
  ('fsvps.gov.ru', 'Россельхознадзор'),
  ('gosnadzor.gov.ru', 'Ростехнадзор'),
  ('rpn.gov.ru', 'Росприроднадзор'),
  ('fedsfm.ru', 'Росфинмониторинг'),

  -- Социальные и публичные сервисы
  ('emias.info', 'ЕМИАС (запись к врачу, Москва)'),
  ('rosminzdrav.ru', 'Минздрав — запись к врачу'),
  ('pension.gov.ru', 'Пенсионный портал'),

  -- Реестры и базы
  ('egrul.nalog.ru', 'ЕГРЮЛ/ЕГРИП'),

  -- Образование и наука
  ('obrnadzor.gov.ru', 'Рособрнадзор'),
  ('ege.edu.ru', 'ЕГЭ'),
  ('fipi.ru', 'ФИПИ'),
  ('school.mosreg.ru', 'Школьный портал Московской области'),

  -- Красноярский край
  ('sibgenco.services', 'Сибирская генерирующая компания'),
  ('krk.sibgenco.services', 'Сибирская генерирующая компания'),
  ('sibgenco.ru', 'Сибирская генерирующая компания'),
  ('rosttech.online', 'РОСТтех'),
  ('krsk-sbit.ru', 'ПАО «РусГидро»'),
  ('sm-city.ru', 'См Сити')
ON CONFLICT (domain) DO NOTHING;
