CREATE TABLE IF NOT EXISTS allowed_domains (
   id SERIAL PRIMARY KEY,
   domain VARCHAR(255) NOT NULL UNIQUE,
   description VARCHAR(255),
   is_active BOOLEAN DEFAULT TRUE,
   created_at TIMESTAMP DEFAULT NOW()
);

-- Начальные данные
INSERT INTO allowed_domains (domain, description) VALUES
  ('ozon.ru', 'Ozon'),
  ('ozon.com', 'Ozon'),
  ('wildberries.ru', 'Wildberries'),
  ('wb.ru', 'Wildberries'),
  ('aliexpress.ru', 'AliExpress'),
  ('aliexpress.com', 'AliExpress'),
  ('market.yandex.ru', 'Яндекс.Маркет'),
  ('ya.ru', 'Яндекс'),
  ('avito.ru', 'Авито'),
  ('youla.ru', 'Юла'),
  ('lamoda.ru', 'Lamoda'),
  ('dns-shop.ru', 'DNS'),
  ('mvideo.ru', 'М.Видео'),
  ('eldorado.ru', 'Эльдорадо'),
  ('citilink.ru', 'Ситилинк'),
  ('sbermegamarket.ru', 'СберМегаМаркет'),
  ('kazanexpress.ru', 'KazanExpress'),
  ('youtube.com', 'YouTube'),
  ('youtu.be', 'YouTube'),
  ('rutube.ru', 'Rutube')
ON CONFLICT (domain) DO NOTHING;
