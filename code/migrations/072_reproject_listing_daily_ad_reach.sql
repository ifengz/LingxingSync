-- Reproject existing advertising raw rows after they have been retained.
-- Missing raw rows remain NULL; no fact rows are deleted or rebuilt.
UPDATE listing_daily_metrics m
JOIN listing_dimensions d ON d.id = m.listing_dimension_id
JOIN ls_stores s ON s.sid = d.store_id
JOIN (
    SELECT account_id, sid, report_date, asin, sku,
           SUM(impressions) AS impressions, SUM(clicks) AS clicks
      FROM ls_ad_sp_product
     GROUP BY account_id, sid, report_date, asin, sku
) raw ON raw.account_id = s.account_id AND raw.sid = d.store_id
     AND raw.report_date = m.business_date AND raw.asin = d.asin AND raw.sku = d.sku
   SET m.sp_impressions = raw.impressions,
       m.sp_impressions_source = CASE WHEN raw.impressions IS NULL THEN m.sp_impressions_source ELSE 'api' END,
       m.sp_clicks = raw.clicks,
       m.sp_clicks_source = CASE WHEN raw.clicks IS NULL THEN m.sp_clicks_source ELSE 'api' END
 WHERE d.channel = 'sc_fba' AND d.identity_scope = 'listing';

UPDATE listing_daily_metrics m
JOIN listing_dimensions d ON d.id = m.listing_dimension_id
JOIN ls_stores s ON s.sid = d.store_id
JOIN (
    SELECT account_id, sid, report_date, asin, sku,
           SUM(impressions) AS impressions, SUM(clicks) AS clicks
      FROM ls_ad_sd_product
     GROUP BY account_id, sid, report_date, asin, sku
) raw ON raw.account_id = s.account_id AND raw.sid = d.store_id
     AND raw.report_date = m.business_date AND raw.asin = d.asin AND raw.sku = d.sku
   SET m.sd_impressions = raw.impressions,
       m.sd_impressions_source = CASE WHEN raw.impressions IS NULL THEN m.sd_impressions_source ELSE 'api' END,
       m.sd_clicks = raw.clicks,
       m.sd_clicks_source = CASE WHEN raw.clicks IS NULL THEN m.sd_clicks_source ELSE 'api' END
 WHERE d.channel = 'sc_fba' AND d.identity_scope = 'listing';

UPDATE listing_daily_metrics m
JOIN listing_dimensions d ON d.id = m.listing_dimension_id
JOIN ls_stores s ON s.sid = d.store_id
JOIN (
    SELECT account_id, sid, report_date,
           SUM(impressions) AS impressions, SUM(clicks) AS clicks
      FROM ls_ad_hsa_campaign
     GROUP BY account_id, sid, report_date
) raw ON raw.account_id = s.account_id AND raw.sid = d.store_id
     AND raw.report_date = m.business_date
   SET m.hsa_impressions = raw.impressions,
       m.hsa_impressions_source = CASE WHEN raw.impressions IS NULL THEN m.hsa_impressions_source ELSE 'api' END,
       m.hsa_clicks = raw.clicks,
       m.hsa_clicks_source = CASE WHEN raw.clicks IS NULL THEN m.hsa_clicks_source ELSE 'api' END
 WHERE d.channel = 'hsa' AND d.identity_scope = 'store';
