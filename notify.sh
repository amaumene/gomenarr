#!/busybox sh
### NZBGET POST-PROCESSING SCRIPT

# Replace this with the URL of your Gomenarr API
# If running in Docker/Podman, use the container name
# If running on host, use localhost:3000
API_URL="http://gomenarr:3000/api/notify"

# Map NZBGet status to SUCCESS/FAILURE
STATUS="FAILURE"
if [ "${NZBPP_TOTALSTATUS}" = "SUCCESS" ]; then
    STATUS="SUCCESS"
fi

# Build query string with proper parameter names
# Use NZBPP_FINALDIR (final extraction directory) not NZBPP_DIRECTORY (temp download dir)
QUERY="status=${STATUS}&name=${NZBPP_NZBNAME}&path=${NZBPP_FINALDIR}&nzbid=${NZBPP_NZBID}&trakt=${NZBPR_TRAKT}"

# Send notification via HTTP GET with query parameters
/busybox wget -q -O - "${API_URL}?${QUERY}"

# Exit with status 93 (POSTPROCESS_SUCCESS) to tell NZBGet we succeeded
exit 93
