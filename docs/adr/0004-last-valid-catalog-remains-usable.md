# The last valid catalog remains usable

After the proxy loads a valid catalog, refresh failures never make that catalog
unusable solely because of age. The proxy reports snapshot age and refresh
errors, but keeps routing with the last valid data. Legacy routing is used only
when no valid catalog has ever loaded, favoring request availability over
catalog freshness.
