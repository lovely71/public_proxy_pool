package sources

import (
	"bufio"
	"strconv"
	"strings"
)

const builtInGitHubSourcesTSV = `topchina-readme	https://raw.githubusercontent.com/TopChina/proxy-list/main/README.md	topchina	http	https://github.com/TopChina/proxy-list	README 表格维护，持续更新	true	3600
proxyscraper-http	https://raw.githubusercontent.com/ProxyScraper/ProxyScraper/main/http.txt	generic	http	https://github.com/ProxyScraper/ProxyScraper	公开说明约每 30 分钟更新	true	3600
proxyscraper-socks4	https://raw.githubusercontent.com/ProxyScraper/ProxyScraper/main/socks4.txt	generic	socks4	https://github.com/ProxyScraper/ProxyScraper	公开说明约每 30 分钟更新	true	3600
proxyscraper-socks5	https://raw.githubusercontent.com/ProxyScraper/ProxyScraper/main/socks5.txt	generic	socks5	https://github.com/ProxyScraper/ProxyScraper	公开说明约每 30 分钟更新	true	3600
monosans-http	https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/http.txt	generic	http	https://github.com/monosans/proxy-list	公开说明每小时更新	true	3600
monosans-socks4	https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/socks4.txt	generic	socks4	https://github.com/monosans/proxy-list	公开说明每小时更新	true	3600
monosans-socks5	https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/socks5.txt	generic	socks5	https://github.com/monosans/proxy-list	公开说明每小时更新	true	3600
proxifly-http	https://raw.githubusercontent.com/proxifly/free-proxy-list/main/proxies/protocols/http/data.txt	generic	http	https://github.com/proxifly/free-proxy-list	公开说明约每 5 分钟更新	true	3600
proxifly-https	https://raw.githubusercontent.com/proxifly/free-proxy-list/main/proxies/protocols/https/data.txt	generic	https	https://github.com/proxifly/free-proxy-list	公开说明约每 5 分钟更新	true	3600
proxifly-socks4	https://raw.githubusercontent.com/proxifly/free-proxy-list/main/proxies/protocols/socks4/data.txt	generic	socks4	https://github.com/proxifly/free-proxy-list	公开说明约每 5 分钟更新	true	3600
proxifly-socks5	https://raw.githubusercontent.com/proxifly/free-proxy-list/main/proxies/protocols/socks5/data.txt	generic	socks5	https://github.com/proxifly/free-proxy-list	公开说明约每 5 分钟更新	true	3600
speedx-http	https://raw.githubusercontent.com/TheSpeedX/SOCKS-List/master/http.txt	generic	http	https://github.com/TheSpeedX/SOCKS-List	公开说明每日更新	true	3600
speedx-socks4	https://raw.githubusercontent.com/TheSpeedX/SOCKS-List/master/socks4.txt	generic	socks4	https://github.com/TheSpeedX/SOCKS-List	公开说明每日更新	true	3600
speedx-socks5	https://raw.githubusercontent.com/TheSpeedX/SOCKS-List/master/socks5.txt	generic	socks5	https://github.com/TheSpeedX/SOCKS-List	公开说明每日更新	true	3600
mmpx12-http	https://raw.githubusercontent.com/mmpx12/proxy-list/master/http.txt	generic	http	https://github.com/mmpx12/proxy-list	常见持续更新源	true	3600
mmpx12-https	https://raw.githubusercontent.com/mmpx12/proxy-list/master/https.txt	generic	https	https://github.com/mmpx12/proxy-list	常见持续更新源	true	3600
mmpx12-socks4	https://raw.githubusercontent.com/mmpx12/proxy-list/master/socks4.txt	generic	socks4	https://github.com/mmpx12/proxy-list	常见持续更新源	true	3600
mmpx12-socks5	https://raw.githubusercontent.com/mmpx12/proxy-list/master/socks5.txt	generic	socks5	https://github.com/mmpx12/proxy-list	常见持续更新源	true	3600
roosterkid-https	https://raw.githubusercontent.com/roosterkid/openproxylist/main/HTTPS_RAW.txt	generic	https	https://github.com/roosterkid/openproxylist	常见持续更新源	true	3600
roosterkid-socks4	https://raw.githubusercontent.com/roosterkid/openproxylist/main/SOCKS4_RAW.txt	generic	socks4	https://github.com/roosterkid/openproxylist	常见持续更新源	true	3600
roosterkid-socks5	https://raw.githubusercontent.com/roosterkid/openproxylist/main/SOCKS5_RAW.txt	generic	socks5	https://github.com/roosterkid/openproxylist	常见持续更新源	true	3600
kangproxy-raw	https://raw.githubusercontent.com/officialputuid/KangProxy/KangProxy/master/xResults/RAW.txt	generic	http	https://github.com/officialputuid/KangProxy	公开说明日更	true	3600
r00tee-https	https://raw.githubusercontent.com/r00tee/Proxy-List/main/Https.txt	generic	https	https://github.com/r00tee/Proxy-List	公开说明约每 5 分钟更新	true	3600
r00tee-socks4	https://raw.githubusercontent.com/r00tee/Proxy-List/main/Socks4.txt	generic	socks4	https://github.com/r00tee/Proxy-List	公开说明约每 5 分钟更新	true	3600
r00tee-socks5	https://raw.githubusercontent.com/r00tee/Proxy-List/main/Socks5.txt	generic	socks5	https://github.com/r00tee/Proxy-List	公开说明约每 5 分钟更新	true	3600
dpangestuw-http	https://raw.githubusercontent.com/dpangestuw/Free-Proxy/main/http_proxies.txt	generic	http	https://github.com/dpangestuw/Free-Proxy	常见持续更新源	true	3600
dpangestuw-https	https://raw.githubusercontent.com/dpangestuw/Free-Proxy/main/https_proxies.txt	generic	https	https://github.com/dpangestuw/Free-Proxy	常见持续更新源	true	3600
dpangestuw-socks4	https://raw.githubusercontent.com/dpangestuw/Free-Proxy/main/socks4_proxies.txt	generic	socks4	https://github.com/dpangestuw/Free-Proxy	常见持续更新源	true	3600
dpangestuw-socks5	https://raw.githubusercontent.com/dpangestuw/Free-Proxy/main/socks5_proxies.txt	generic	socks5	https://github.com/dpangestuw/Free-Proxy	常见持续更新源	true	3600
sunny9577-all	https://raw.githubusercontent.com/sunny9577/proxy-scraper/master/proxies.txt	generic	http	https://github.com/sunny9577/proxy-scraper	公开说明每日更新	true	3600
thenasty1337-latest	https://raw.githubusercontent.com/thenasty1337/free-proxy-list/main/data/latest/proxies.txt	generic	http	https://github.com/thenasty1337/free-proxy-list	公开说明最新快照	true	3600
vakhov-http	https://raw.githubusercontent.com/vakhov/fresh-proxy-list/master/http.txt	generic	http	https://github.com/vakhov/fresh-proxy-list	公开说明约每 5-20 分钟更新	true	3600
vakhov-https	https://raw.githubusercontent.com/vakhov/fresh-proxy-list/master/https.txt	generic	https	https://github.com/vakhov/fresh-proxy-list	公开说明约每 5-20 分钟更新	true	3600
vakhov-socks4	https://raw.githubusercontent.com/vakhov/fresh-proxy-list/master/socks4.txt	generic	socks4	https://github.com/vakhov/fresh-proxy-list	公开说明约每 5-20 分钟更新	true	3600
vakhov-socks5	https://raw.githubusercontent.com/vakhov/fresh-proxy-list/master/socks5.txt	generic	socks5	https://github.com/vakhov/fresh-proxy-list	公开说明约每 5-20 分钟更新	true	3600
fyvri-http	https://raw.githubusercontent.com/fyvri/fresh-proxy-list/main/http.txt	generic	http	https://github.com/fyvri/fresh-proxy-list	公开说明约每 60 分钟更新	true	3600
fyvri-https	https://raw.githubusercontent.com/fyvri/fresh-proxy-list/main/https.txt	generic	https	https://github.com/fyvri/fresh-proxy-list	公开说明约每 60 分钟更新	true	3600
fyvri-socks4	https://raw.githubusercontent.com/fyvri/fresh-proxy-list/main/socks4.txt	generic	socks4	https://github.com/fyvri/fresh-proxy-list	公开说明约每 60 分钟更新	true	3600
fyvri-socks5	https://raw.githubusercontent.com/fyvri/fresh-proxy-list/main/socks5.txt	generic	socks5	https://github.com/fyvri/fresh-proxy-list	公开说明约每 60 分钟更新	true	3600
aliilapro-http	https://raw.githubusercontent.com/ALIILAPRO/Proxy/main/http.txt	generic	http	https://github.com/ALIILAPRO/Proxy	公开说明每小时更新	true	3600
aliilapro-socks4	https://raw.githubusercontent.com/ALIILAPRO/Proxy/main/socks4.txt	generic	socks4	https://github.com/ALIILAPRO/Proxy	公开说明每小时更新	true	3600
aliilapro-socks5	https://raw.githubusercontent.com/ALIILAPRO/Proxy/main/socks5.txt	generic	socks5	https://github.com/ALIILAPRO/Proxy	公开说明每小时更新	true	3600
iplocate-http	https://raw.githubusercontent.com/iplocate/free-proxy-list/main/http.txt	generic	http	https://github.com/iplocate/free-proxy-list	公开说明约每 30 分钟更新	true	3600
iplocate-https	https://raw.githubusercontent.com/iplocate/free-proxy-list/main/https.txt	generic	https	https://github.com/iplocate/free-proxy-list	公开说明约每 30 分钟更新	true	3600
iplocate-socks4	https://raw.githubusercontent.com/iplocate/free-proxy-list/main/socks4.txt	generic	socks4	https://github.com/iplocate/free-proxy-list	公开说明约每 30 分钟更新	true	3600
iplocate-socks5	https://raw.githubusercontent.com/iplocate/free-proxy-list/main/socks5.txt	generic	socks5	https://github.com/iplocate/free-proxy-list	公开说明约每 30 分钟更新	true	3600
ercin-http	https://raw.githubusercontent.com/ErcinDedeoglu/proxies/main/proxies/http.txt	generic	http	https://github.com/ErcinDedeoglu/proxies	公开说明约每 60 分钟更新	true	3600
ercin-https	https://raw.githubusercontent.com/ErcinDedeoglu/proxies/main/proxies/https.txt	generic	https	https://github.com/ErcinDedeoglu/proxies	公开说明约每 60 分钟更新	true	3600
ercin-socks4	https://raw.githubusercontent.com/ErcinDedeoglu/proxies/main/proxies/socks4.txt	generic	socks4	https://github.com/ErcinDedeoglu/proxies	公开说明约每 60 分钟更新	true	3600
ercin-socks5	https://raw.githubusercontent.com/ErcinDedeoglu/proxies/main/proxies/socks5.txt	generic	socks5	https://github.com/ErcinDedeoglu/proxies	公开说明约每 60 分钟更新	true	3600
shiftytr-http	https://raw.githubusercontent.com/ShiftyTR/Proxy-List/main/http.txt	generic	http	https://github.com/ShiftyTR/Proxy-List	公开说明每小时更新	true	3600
shiftytr-https	https://raw.githubusercontent.com/ShiftyTR/Proxy-List/main/https.txt	generic	https	https://github.com/ShiftyTR/Proxy-List	公开说明每小时更新	true	3600
shiftytr-socks4	https://raw.githubusercontent.com/ShiftyTR/Proxy-List/main/socks4.txt	generic	socks4	https://github.com/ShiftyTR/Proxy-List	公开说明每小时更新	true	3600
shiftytr-socks5	https://raw.githubusercontent.com/ShiftyTR/Proxy-List/main/socks5.txt	generic	socks5	https://github.com/ShiftyTR/Proxy-List	公开说明每小时更新	true	3600
skillter-http	https://raw.githubusercontent.com/Skillter/ProxyGather/main/proxies/working-proxies-http.txt	generic	http	https://github.com/Skillter/ProxyGather	公开说明约每 30 分钟更新	true	3600
skillter-socks4	https://raw.githubusercontent.com/Skillter/ProxyGather/main/proxies/working-proxies-socks4.txt	generic	socks4	https://github.com/Skillter/ProxyGather	公开说明约每 30 分钟更新	true	3600
skillter-socks5	https://raw.githubusercontent.com/Skillter/ProxyGather/main/proxies/working-proxies-socks5.txt	generic	socks5	https://github.com/Skillter/ProxyGather	公开说明约每 30 分钟更新	true	3600
zloi-http	https://raw.githubusercontent.com/zloi-user/hideip.me/master/http.txt	generic	http	https://github.com/zloi-user/hideip.me	公开说明约每 10 分钟更新	true	3600
zloi-https	https://raw.githubusercontent.com/zloi-user/hideip.me/master/https.txt	generic	https	https://github.com/zloi-user/hideip.me	公开说明约每 10 分钟更新	true	3600
zloi-socks4	https://raw.githubusercontent.com/zloi-user/hideip.me/master/socks4.txt	generic	socks4	https://github.com/zloi-user/hideip.me	公开说明约每 10 分钟更新	true	3600
zloi-socks5	https://raw.githubusercontent.com/zloi-user/hideip.me/master/socks5.txt	generic	socks5	https://github.com/zloi-user/hideip.me	公开说明约每 10 分钟更新	true	3600
zloi-connect	https://raw.githubusercontent.com/zloi-user/hideip.me/master/connect.txt	generic	http	https://github.com/zloi-user/hideip.me	公开说明约每 10 分钟更新	true	3600
zaeem20-http	https://raw.githubusercontent.com/Zaeem20/FREE_PROXIES_LIST/master/http.txt	generic	http	https://github.com/Zaeem20/FREE_PROXIES_LIST	公开说明约每 10 分钟更新	true	3600
zaeem20-https	https://raw.githubusercontent.com/Zaeem20/FREE_PROXIES_LIST/master/https.txt	generic	https	https://github.com/Zaeem20/FREE_PROXIES_LIST	公开说明约每 10 分钟更新	true	3600
zaeem20-socks4	https://raw.githubusercontent.com/Zaeem20/FREE_PROXIES_LIST/master/socks4.txt	generic	socks4	https://github.com/Zaeem20/FREE_PROXIES_LIST	公开说明约每 10 分钟更新	true	3600
zaeem20-socks5	https://raw.githubusercontent.com/Zaeem20/FREE_PROXIES_LIST/master/socks5.txt	generic	socks5	https://github.com/Zaeem20/FREE_PROXIES_LIST	公开说明约每 10 分钟更新	true	3600
argh94-http	https://raw.githubusercontent.com/Argh94/Proxy-List/main/HTTP.txt	generic	http	https://github.com/Argh94/Proxy-List	公开说明每小时更新	true	3600
argh94-https	https://raw.githubusercontent.com/Argh94/Proxy-List/main/HTTPS.txt	generic	https	https://github.com/Argh94/Proxy-List	公开说明每小时更新	true	3600
argh94-socks4	https://raw.githubusercontent.com/Argh94/Proxy-List/main/SOCKS4.txt	generic	socks4	https://github.com/Argh94/Proxy-List	公开说明每小时更新	true	3600
argh94-socks5	https://raw.githubusercontent.com/Argh94/Proxy-List/main/SOCKS5.txt	generic	socks5	https://github.com/Argh94/Proxy-List	公开说明每小时更新	true	3600
themiralay-data	https://raw.githubusercontent.com/themiralay/Proxy-List-World/master/data.txt	generic	http	https://github.com/themiralay/Proxy-List-World	公开说明约每 10 分钟更新	true	3600
gfpcom-http	https://raw.githubusercontent.com/wiki/gfpcom/free-proxy-list/lists/http.txt	generic	http	https://github.com/gfpcom/free-proxy-list	公开说明约每 30 分钟更新	true	3600
gfpcom-https	https://raw.githubusercontent.com/wiki/gfpcom/free-proxy-list/lists/https.txt	generic	https	https://github.com/gfpcom/free-proxy-list	公开说明约每 30 分钟更新	true	3600
gfpcom-socks4	https://raw.githubusercontent.com/wiki/gfpcom/free-proxy-list/lists/socks4.txt	generic	socks4	https://github.com/gfpcom/free-proxy-list	公开说明约每 30 分钟更新	true	3600
gfpcom-socks5	https://raw.githubusercontent.com/wiki/gfpcom/free-proxy-list/lists/socks5.txt	generic	socks5	https://github.com/gfpcom/free-proxy-list	公开说明约每 30 分钟更新	true	3600
kangproxy-http	https://raw.githubusercontent.com/officialputuid/KangProxy/KangProxy/http/http.txt	generic	http	https://github.com/officialputuid/KangProxy	公开说明日更	true	3600
kangproxy-https	https://raw.githubusercontent.com/officialputuid/KangProxy/KangProxy/https/https.txt	generic	https	https://github.com/officialputuid/KangProxy	公开说明日更	true	3600
kangproxy-socks4	https://raw.githubusercontent.com/officialputuid/KangProxy/KangProxy/socks4/socks4.txt	generic	socks4	https://github.com/officialputuid/KangProxy	公开说明日更	true	3600
kangproxy-socks5	https://raw.githubusercontent.com/officialputuid/KangProxy/KangProxy/socks5/socks5.txt	generic	socks5	https://github.com/officialputuid/KangProxy	公开说明日更	true	3600
sunny9577-http	https://raw.githubusercontent.com/sunny9577/proxy-scraper/master/generated/http_proxies.txt	generic	http	https://github.com/sunny9577/proxy-scraper	公开说明约每 3 小时更新	true	3600
sunny9577-socks4	https://raw.githubusercontent.com/sunny9577/proxy-scraper/master/generated/socks4_proxies.txt	generic	socks4	https://github.com/sunny9577/proxy-scraper	公开说明约每 3 小时更新	true	3600
sunny9577-socks5	https://raw.githubusercontent.com/sunny9577/proxy-scraper/master/generated/socks5_proxies.txt	generic	socks5	https://github.com/sunny9577/proxy-scraper	公开说明约每 3 小时更新	true	3600
thenasty1337-http	https://raw.githubusercontent.com/thenasty1337/free-proxy-list/main/data/latest/types/http/proxies.txt	generic	http	https://github.com/thenasty1337/free-proxy-list	公开说明约每 6 小时更新	true	3600
thenasty1337-socks4	https://raw.githubusercontent.com/thenasty1337/free-proxy-list/main/data/latest/types/socks4/proxies.txt	generic	socks4	https://github.com/thenasty1337/free-proxy-list	公开说明约每 6 小时更新	true	3600
thenasty1337-socks5	https://raw.githubusercontent.com/thenasty1337/free-proxy-list/main/data/latest/types/socks5/proxies.txt	generic	socks5	https://github.com/thenasty1337/free-proxy-list	公开说明约每 6 小时更新	true	3600
murongpig-http	https://raw.githubusercontent.com/MuRongPIG/Proxy-Master/main/http.txt	generic	http	https://github.com/MuRongPIG/Proxy-Master	社区常用源	true	3600
prxchk-http	https://raw.githubusercontent.com/prxchk/proxy-list/main/http.txt	generic	http	https://github.com/prxchk/proxy-list	社区常用源	true	3600
b4rcode-http	https://raw.githubusercontent.com/B4RC0DE-TM/proxy-list/main/HTTP.txt	generic	http	https://github.com/B4RC0DE-TM/proxy-list	社区常用源	true	3600
saschazesiger-http	https://raw.githubusercontent.com/saschazesiger/Free-Proxies/master/proxies/http.txt	generic	http	https://github.com/saschazesiger/Free-Proxies	社区常用源	true	3600
saschazesiger-socks4	https://raw.githubusercontent.com/saschazesiger/Free-Proxies/master/proxies/socks4.txt	generic	socks4	https://github.com/saschazesiger/Free-Proxies	社区常用源	true	3600
saschazesiger-socks5	https://raw.githubusercontent.com/saschazesiger/Free-Proxies/master/proxies/socks5.txt	generic	socks5	https://github.com/saschazesiger/Free-Proxies	社区常用源	true	3600
hyperbeats-http	https://raw.githubusercontent.com/HyperBeats/proxy-list/main/http.txt	generic	http	https://github.com/HyperBeats/proxy-list	社区常用源	true	3600
hyperbeats-socks4	https://raw.githubusercontent.com/HyperBeats/proxy-list/main/socks4.txt	generic	socks4	https://github.com/HyperBeats/proxy-list	社区常用源	true	3600
hyperbeats-socks5	https://raw.githubusercontent.com/HyperBeats/proxy-list/main/socks5.txt	generic	socks5	https://github.com/HyperBeats/proxy-list	社区常用源	true	3600
jetkai-http	https://raw.githubusercontent.com/jetkai/proxy-list/main/online-proxies/txt/proxies-http.txt	generic	http	https://github.com/jetkai/proxy-list	社区常用源	true	3600
jetkai-socks4	https://raw.githubusercontent.com/jetkai/proxy-list/main/online-proxies/txt/proxies-socks4.txt	generic	socks4	https://github.com/jetkai/proxy-list	社区常用源	true	3600
jetkai-socks5	https://raw.githubusercontent.com/jetkai/proxy-list/main/online-proxies/txt/proxies-socks5.txt	generic	socks5	https://github.com/jetkai/proxy-list	社区常用源	true	3600
rdavydov-http	https://raw.githubusercontent.com/rdavydov/proxy-list/main/proxies/http.txt	generic	http	https://github.com/rdavydov/proxy-list	社区常用源	true	3600
rdavydov-anon-http	https://raw.githubusercontent.com/rdavydov/proxy-list/main/proxies_anonymous/http.txt	generic	http	https://github.com/rdavydov/proxy-list	社区常用源	true	3600
clarketm-raw	https://raw.githubusercontent.com/clarketm/proxy-list/master/proxy-list-raw.txt	generic	http	https://github.com/clarketm/proxy-list	社区常用源	true	3600
opsxcq-list	https://raw.githubusercontent.com/opsxcq/proxy-list/master/list.txt	generic	http	https://github.com/opsxcq/proxy-list	社区常用源	true	3600
speedx-proxylist-http	https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/http.txt	generic	http	https://github.com/TheSpeedX/PROXY-List	公开说明每小时更新	true	3600
speedx-proxylist-socks4	https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/socks4.txt	generic	socks4	https://github.com/TheSpeedX/PROXY-List	公开说明每小时更新	true	3600
speedx-proxylist-socks5	https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/socks5.txt	generic	socks5	https://github.com/TheSpeedX/PROXY-List	公开说明每小时更新	true	3600
hookzof-socks5	https://raw.githubusercontent.com/hookzof/socks5_list/master/proxy.txt	generic	socks5	https://github.com/hookzof/socks5_list	社区常用源	true	3600
proxify-http	https://raw.githubusercontent.com/Firmfox/proxify/main/proxies/http.txt	generic	http	https://github.com/Firmfox/Proxify	公开说明约每 30 分钟更新	true	3600
proxify-https	https://raw.githubusercontent.com/Firmfox/proxify/main/proxies/https.txt	generic	https	https://github.com/Firmfox/Proxify	公开说明约每 30 分钟更新	true	3600
proxify-socks4	https://raw.githubusercontent.com/Firmfox/proxify/main/proxies/socks4.txt	generic	socks4	https://github.com/Firmfox/Proxify	公开说明约每 30 分钟更新	true	3600
proxify-socks5	https://raw.githubusercontent.com/Firmfox/proxify/main/proxies/socks5.txt	generic	socks5	https://github.com/Firmfox/Proxify	公开说明约每 30 分钟更新	true	3600
loneking-all	https://raw.githubusercontent.com/LoneKingCode/free-proxy-db/refs/heads/main/proxies/all.txt	generic	http	https://github.com/LoneKingCode/free-proxy-db	公开说明每小时更新	true	3600
loneking-http	https://raw.githubusercontent.com/LoneKingCode/free-proxy-db/refs/heads/main/proxies/http.txt	generic	http	https://github.com/LoneKingCode/free-proxy-db	公开说明每小时更新	true	3600
loneking-socks4	https://raw.githubusercontent.com/LoneKingCode/free-proxy-db/refs/heads/main/proxies/socks4.txt	generic	socks4	https://github.com/LoneKingCode/free-proxy-db	公开说明每小时更新	true	3600
loneking-socks5	https://raw.githubusercontent.com/LoneKingCode/free-proxy-db/refs/heads/main/proxies/socks5.txt	generic	socks5	https://github.com/LoneKingCode/free-proxy-db	公开说明每小时更新	true	3600
monosans-anon-http	https://raw.githubusercontent.com/monosans/proxy-list/main/proxies_anonymous/http.txt	generic	http	https://github.com/monosans/proxy-list	公开说明每小时更新（匿名代理）	true	3600
monosans-anon-socks4	https://raw.githubusercontent.com/monosans/proxy-list/main/proxies_anonymous/socks4.txt	generic	socks4	https://github.com/monosans/proxy-list	公开说明每小时更新（匿名代理）	true	3600
monosans-anon-socks5	https://raw.githubusercontent.com/monosans/proxy-list/main/proxies_anonymous/socks5.txt	generic	socks5	https://github.com/monosans/proxy-list	公开说明每小时更新（匿名代理）	true	3600
proxifly-all	https://raw.githubusercontent.com/proxifly/free-proxy-list/main/proxies/all/data.txt	generic	http	https://github.com/proxifly/free-proxy-list	公开说明约每 5 分钟更新	true	3600
dpangestuw-allive	https://raw.githubusercontent.com/dpangestuw/Free-Proxy/main/allive_proxies.txt	generic	http	https://github.com/dpangestuw/Free-Proxy	常见持续更新源（存活合集）	true	3600
roosterkid-https-rich	https://raw.githubusercontent.com/roosterkid/openproxylist/main/HTTPS.txt	generic	https	https://github.com/roosterkid/openproxylist	常见持续更新源	true	3600
roosterkid-socks4-rich	https://raw.githubusercontent.com/roosterkid/openproxylist/main/SOCKS4.txt	generic	socks4	https://github.com/roosterkid/openproxylist	常见持续更新源	true	3600
roosterkid-socks5-rich	https://raw.githubusercontent.com/roosterkid/openproxylist/main/SOCKS5.txt	generic	socks5	https://github.com/roosterkid/openproxylist	常见持续更新源	true	3600
mishakorzik-freeproxy	https://raw.githubusercontent.com/mishakorzik/Free-Proxy/main/proxy.txt	generic	http	https://github.com/mishakorzik/Free-Proxy	社区常用源	true	3600`

func BuiltInGitHubSources() []SourceDef {
	scanner := bufio.NewScanner(strings.NewReader(builtInGitHubSourcesTSV))
	out := make([]SourceDef, 0, 128)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 8 {
			continue
		}
		enabled := strings.EqualFold(parts[6], "true")
		intervalSec, err := strconv.Atoi(parts[7])
		if err != nil {
			intervalSec = 3600
		}
		out = append(out, SourceDef{
			Name:          parts[0],
			Type:          "github_raw_text",
			URL:           parts[1],
			Parser:        parts[2],
			DefaultScheme: parts[3],
			RepoURL:       parts[4],
			UpdateHint:    parts[5],
			Enabled:       enabled,
			IntervalSec:   intervalSec,
		})
	}
	return out
}
