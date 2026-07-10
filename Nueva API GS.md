**<font style="color:rgb(0,0,0);">Payment Open API </font>****Document v2.0**



| Document No. | GSWYIT-OPEN-API-02 |  |  |  |  |
| :---: | :---: | --- | --- | --- | --- |
| Number of pages |  | Modify state |  | Effective date |  |
| Proposed |  | To examine |  | Approval |  |


**(All rights reserved. Reproduction will be prosecuted)**



Modify history



| Version | Modify   identification | Reason/content for modification | Modified by | Reviewer | Approver | Modify time |
| --- | :---: | --- | :---: | :---: | --- | --- |
| V1.0 | New-built | Create original document | Cy zhao |  |  | 2024-12-11 |
| V2.0 | <font style="color:rgb(49, 49, 49);">Update </font> | Modifie document | Cy zhao |  |  | 2026-02-26 |
|  |  |  |  |  |  |  |
|  |  |  |  |  |  |  |
|  |  |  |  |  |  |  |
|  |  |  |  |  |  |  |


## business process diagram
<img src="https://cdn.nlark.com/yuque/0/2026/png/65921018/1771984375363-2909f773-52ce-46fe-8dff-39b9fa07354c.png" width="1112" title="null" crop="0,0,1,1" id="HPQjf" class="ne-image">

Descriptiion: GS provides Android and server-side services to facilitate order transactions and beverage preparation processes.  



## Access requirements
### Platform account information
How To get GS platform account information?

+ Login to the Gs-vending Management platform
+ Visit address: [https://gsvden.coffeeji.com](https://gsvden.coffeeji.com)
+ Login account provide by Gs-vending
+ Switch to the user information page
+ <img src="https://cdn.nlark.com/yuque/0/2026/png/65921018/1771984714913-844bc793-34df-4c1c-afd5-45e526d5df19.png" width="1118" title="null" crop="0,0,1,1" id="NL4Wo" class="ne-image">
+ Obtain the encryption configuration information of the interface request  
+ <img src="https://cdn.nlark.com/yuque/0/2026/png/65921018/1771984738208-bccec319-0512-4a4d-97bc-b3d087cf065a.png" width="1106" title="null" crop="0,0,1,1" id="W6LY1" class="ne-image">

### third party <font style="color:rgb(49, 49, 49);">needs to provide</font>
| domain | + <font style="color:rgb(49, 49, 49);background-color:rgb(244, 244, 244);">api interface domain name</font> |
| --- | --- |


## Interface design
### Access guide
#### Request protocol rules
Transmission method: To ensure transaction security, the Content-type is all application/json Request  submission method: post  

| Content-type | application/json |
| --- | --- |
| Method | post |


#### Data structure
 Both the submitted and returned data are in JSON format Uniformly adopt UTF-8 character encoding

### Request public parameters
#### Public request header parameters
Description: The request header of all requests must contain the following authentication information  

Request header parameters:

| name | type | required | notes |
| --- | --- | --- | --- |
| key | String | required | 2.1.Encryption configuration(api-key) in the management platform |
| key-md5 | String | required | MD5(key + secret + timestamp)，key and secret Can be generated in the management backend |
| timestamp | String | required | Timestamp of the Eastern 8th Time Zone(In milliseconds,Example:17527 31459238) |


#### Public request return body
 Description: The response body of all requests must contain the following attributes

 Requested repsonse body attributes:

| attribute | type | required | notes |
| --- | --- | --- | --- |
| `code` | String | required |  |
| `msg` | String | required |  |
| `data` | Object | required |  |


 code description: 

| code | notes |
| --- | --- |
| `200` | success |
| `400` | failed |


### Api Interface
  Third party provider

#### Generate payment QR code
+  Generate transactions through qr payment  

:::tips
+ **URL**：`{{domain}}+/order/qr`
+ **Method**：`post`
+ **sender of the request**：<font style="color:#664900;background-color:#f6e1ac;">Machine server</font>
+ **receiver of the request**：<font style="color:#664900;background-color:#f6e1ac;">Third party</font>

:::

+ Request Data:

| attribute | type | required | notes |
| --- | --- | --- | --- |
| `orderNo` | String | required | Transaction serial no |
| `objectId` | String | optional | Trading commodity ID;<br/>GoodsId |
| `subject` | String | required | Goods Name |
| `attach` | String | optional | Custom parameters for device information, feedback(   deviceNo=E00375&deviceId=7678242eba) |
| `totalAmount` | BigDecimal(10,2) | required | Payment amount |
| `notifyUrl` | String | required | Payment callback address interface |


+ Response Data：

| attribute | type | required | notes |
| --- | --- | --- | --- |
| `qrUrl` | String | yes | QR code link |
| `orderStatus` | int | required | payment status  (1- Pending Payment 2- Payment Successful 3- Transaction Failed 4- Pending Refund 5- Refund Completed 6- Time Exceeded) |
| `thirdOrderNo` | String | required | Third party channel order number |


+ <font style="color:rgb(49, 49, 49);background-color:rgb(244, 244, 244);">Request example</font>

```json
{
  
  "orderNo":"A260604132617314029788",
  "objectId":"2062353942715850753",
  "attach": "deviceNo=E02755&deviceId=6777a57edb253f85",   
  "subject":"Recovery Creatina BPI-5",
  "totalAmount":"1",
  "notifyUrl":"https://gsvden.coffeeji.com/pay/notify/tppay/A260604132617314029788"
}
```

+ <font style="color:rgb(49, 49, 49);">Successful response</font>

```json
{
   "code": 200,
   "msg": "success",
   "data": {
        "qrUrl": "https://proteinas.rocko.me/pay/12333555",
        "orderStatus": "1",
        "thirdOrderNo": "3892432"
    }
}
```

#### Query payment result
Query the result of the payment

:::tips
+ **URL**：`{{domain}}+/order/status`
+ **Method**：`post`
+ **sender of the request**：<font style="color:#664900;background-color:#f6e1ac;">Machine server</font>
+ **receiver of the request**：<font style="color:#664900;background-color:#f6e1ac;">Third party</font>

:::

+ Request Data:

| attribute | type | required | default | notes |
| --- | --- | --- | --- | --- |
| `orderNo` | String | required |  | Transaction serial no |
| `thirdOrderNo` | String | required |  | Third party serial no |


+ Response Data：

| attribute | type | required | notes |
| --- | --- | --- | --- |
| `orderNo` | String | required | Transaction serial no |
| `thirdOrderNo` | String | required | Third party serial no |
| `orderStatus` | String | required | Payment status (1- Pending Payment 2- Payment Successful 3- Transaction Failed 4- Pending Refund 5- Refund Completed 6- Time Exceeded) |


+ <font style="color:rgb(49, 49, 49);background-color:rgb(244, 244, 244);">Request example</font>

```json
{
  "orderNo": "A202507121049430895",
  "thirdOrderNo": "1943865325673693185"
}
```

+ <font style="color:rgb(49, 49, 49);">Successful response</font>

```json
{
  "code": 200,
  "msg": "Order status",
  "data": {
        "orderNo": "A202602010418544519",
        "thirdOrderNo": "2017694096980561922",
        "orderStatus": "1",
        "orderTime": "2026-01-31 20:18:55",
        "payTime": "2026-01-31 20:19:04",
        "totalAmount": "55.00",
        "channelUserId": ""
    }
}
```

#### Refund order
Refund the payment order

:::tips
+ **URL**：`{{domain}}+/order/refund`
+ **Method**：`post`
+ **sender of the request**：<font style="color:#664900;background-color:#f6e1ac;">Machine server</font>
+ **receiver of the request**：<font style="color:#664900;background-color:#f6e1ac;">Third party</font>

:::

+ Request Data:

| attribute | type | required | notes |
| --- | --- | --- | --- |
| `refundNo` | String | required | Refund Transaction serial no |
| `orderNo` | String | required | Transaction serial no |
| `thirdOrderNo` | String | required | Third party serial no |
| `refundAmount` | BigDecimal(10,2) | required | Refund transaction amount |
| `refundReason` | String | optional | Refund transaction reason |


+ Response Data：

| attribute | type | required | notes |
| --- | --- | --- | --- |
| `refundNo` | String | required | Refund serial no |
| `orderNo` | String | required | Transaction serial no |
| `thirdOrderNo` | String | required | Third party serial no |
| `thirdRefundNo` | String | optional | Third party refund serial no |
| `refundStatus` | String | required | Refund status(success ,waiting, fail) |
| `refundMsg` | String | required | Refund status detail(success or fail) |
| `refundTime` | String | optional | Refund time,Time format:yyyy-MM-dd HH:mm:ss |
| `totalAmount` | BigDecimal(10,2) | required | Original order amount |
| `refundAmount` | BigDecimal(10,2) | required | refund amount |


+ <font style="color:rgb(49, 49, 49);background-color:rgb(244, 244, 244);">Request example</font>

```json
{
  "orderNo": "A202507121049430895",
  "thirdOrderNo": "1943865325673693185",
  "refundNo":"RD202507121049430895",
  "refundAmount":"100",
  "refundReason":"Customer cancelled"
}
```

+ <font style="color:rgb(49, 49, 49);">Successful response</font>

```json
{
  "code": 200,
  "msg": "Order status",
  "data": {
        "orderNo": "A202507121049430895",
        "thirdOrderNo": "1943865325673693185",
        "refundNo": "RD202507121049430895",
        "refundAmount": "100",
        "totalAmount": "100",
        "refundStatus": "success",
        "refundMsg": "success",
        "refundTime": "2025-07-12 10:49:57"
    }
}
```

#### **Query refund result**
Query the result of the Refund

:::tips
+ **URL**：`{{domain}}+/order/refundStatus`
+ **Method**：`post`
+ **sender of the request**：<font style="color:#664900;background-color:#f6e1ac;">Machine server</font>
+ **receiver of the request**：<font style="color:#664900;background-color:#f6e1ac;">Third party</font>

:::

+ Request Data:

| attribute | type | required | notes |
| --- | --- | --- | --- |
| `refundNo` | String | optional | Refund Transaction serial no |
| `orderNo` | String | required | Transaction serial no |
| `thirdOrderNo` | String | optional | Original Third party serial no |


+ Response Data：

| attribute | type | required | notes |
| --- | --- | --- | --- |
| `refundNo` | String | required | Refund Transaction serial no |
| `orderNo` | String | required | Transaction serial no |
| `thirdOrderNo` | String | required | Third party serial no |
| `thirdRefundNo` | String | optional | Third party refund serial no |
| `refundStatus` | String | required | Refund status(pending,success or fail) |
| `refundMsg` | String | required | Refund status detail(success or fail) |
| `refundTime` | String | required | Refund time,Time format:yyyy-MM-dd HH:mm:ss |
| `totalAmount` | BigDecimal(10,2) | required | Original order amount |
| `refundAmount` | String | required | refund amount |


+ <font style="color:rgb(49, 49, 49);background-color:rgb(244, 244, 244);">Request example</font>

```json
{
  "orderNo": "A202507121049430895",
  "thirdOrderNo": "1943865325673693185",
  "refundNo":"RD202507121049430895"
}
```

+ <font style="color:rgb(49, 49, 49);">Successful response</font>

```json
{
  "code": 200,
  "msg": "Order status",
  "data": {
        "orderNo": "A202507121049430895",
        "thirdOrderNo": "1943865325673693185",
        "refundNo": "RD202507121049430895",
        "refundAmount": "100",
        "totalAmount": "100",
        "refundStatus": "success",
        "refundMsg": "success",
        "refundTime": "2025-07-12 10:49:57"
    }
}
```

#### **Notify payment result**
Third party notify Machine server payment result  

:::tips
+ **URL**：`The field notifyUrl provided in section 3.3.1`
+ **Method**：`post`
+ **sender of the request**：<font style="color:#664900;background-color:#f6e1ac;">Third party</font>
+ **receiver of the request**：<font style="color:#664900;background-color:#f6e1ac;">Machine server</font>

:::

+ Request data

| attribute | type | required | notes |
| --- | --- | --- | --- |
| `orderNo` | String | required | Transaction serial no |
| `thirdOrderNo` | String | required | Third party serial no |
| `orderStatus` | String | required | Payment status  (1- Pending Payment 2- Payment Successful 3- Transaction Failed 4- Pending Refund 5- Refund Completed 6- Time Exceeded) |
| `orderTime` | String | optional | Generate payment time,Time format:yyyy-MM-dd HH:mm:ss |
| `payTime` | String | optional | Actual payment time,Time format:yyyy-MM-dd HH:mm:ss |
| `totalAmount` | BigDecimal(10,2) | required | Payment amount |
| `channelUserId` | String | optional | Payment user channel user ID, App user identification |


+ Response Data：

| attribute | type | required | notes |
| --- | --- | --- | --- |
| `orderNo` | String | required | Original |
| `thirdOrderNo` | String | required | Third party serial no |
| `returnCode` | String | required | Notify status; success or fail |


+ <font style="color:rgb(49, 49, 49);background-color:rgb(244, 244, 244);">Request example</font>

```json
{
  "orderNo": "A202603020228158619",
  "thirdOrderNo": "2028175498066894849",
  "orderStatus": "2",
  "orderTime": "2026-03-01 18:28:16",
  "payTime": "2026-03-01 18:30:14",
  "totalAmount": "15.00"
}
```

+ <font style="color:rgb(49, 49, 49);">Successful response</font>

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    }
}
```

#### Beverage preparation processes notify
Notify third parties upon completion of beverage preparation

:::tips
+ **URL**：`{{domain}} +/order/complete`
+ **Method**：`post`
+ **sender of the request**：<font style="color:#664900;background-color:#f6e1ac;">Machine server</font>
+ **receiver of the request**：<font style="color:#664900;background-color:#f6e1ac;">Third party</font>

:::

+ Request Data:

| attribute | type | required | notes |
| --- | --- | --- | --- |
| `orderNo` | String | required | Transaction serial no |
| `thirdOrderNo` | String | required | Third party serial no |
| `success` | Boolean | required | The successful status of making beverages(   true:making successed,false:making failed) |
| `orderStatus` | int | required | payment status  (1- Pending Payment 2- Payment Successful 3- Transaction Failed 4- Pending Refund 5- Refund Completed 6- Time Exceeded) |
| `outStockStatus`    | int | required | Delivery status for beverage preparation;1:Not shipped yet,2:Already shipped |
| `outStockTime` | String | optional | Delivery time for beverage preparation,Time format:yyyy-MM-dd HH:mm:ss |


+ Response Data：

| attribute | type | required | notes |
| --- | --- | --- | --- |
| `orderNo` | String | required | Transaction serial no |
| `thirdOrderNo` | String | required | Third party serial no |
| `returnCode` | String | required | Notify status: success or fail |
| `returnMsg` | String | required | Cancel payment reason |


+ <font style="color:rgb(49, 49, 49);background-color:rgb(244, 244, 244);">Request example</font>

```json
{
  "objectId": "1993718663956721666",
  "orderNo": "A202603020921059582",
  "orderStatus": 2,
  "outStockStatus": 2,
  "outStockTime": "2026-03-02 09:23:03",
  "success": true,
  "thirdOrderNo": "148432240734"
}
```

+ <font style="color:rgb(49, 49, 49);">Successful response</font>

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "orderNo": "A202603020921059582",
    "thirdOrderNo": "148432240734",
    "returnCode": "success",
    "returnMsg": "success"
    }
}
```

#### **Cancel payment notification**
Cancel the payment

:::tips
+ **URL**：`{{domain}} +/order/cancel`
+ **Method**：`post`
+ **sender of the request**：<font style="color:#664900;background-color:#f6e1ac;">Machine server</font>
+ **receiver of the request**：<font style="color:#664900;background-color:#f6e1ac;">Third party</font>

:::

+ Request Data:

| attribute | type | required | notes |
| --- | --- | --- | --- |
| `orderNo` | String | required | Transaction serial no |
| `thirdOrderNo` | String | required | Third party serial no |
| `orderStatus` | String | required | payment status  (0- Cancel Payment) |
| `remark` | String | optional | Cancel payment reason |
| `cancelTime` | String | required | Time time for cancel payment preparation,Time format:yyyy-MM-dd HH:mm:ss |


+ Response Data：

| attribute | type | required | notes |
| --- | --- | --- | --- |
| `orderNo` | String | required | Transaction serial no |
| `thirdOrderNo` | String | required | Third party serial no |
| `returnCode` | String | required | Cancel payment status; success or fail |
| `returnMsg` | String | required | Cancel payment reason |


+ <font style="color:rgb(49, 49, 49);background-color:rgb(244, 244, 244);">Request example</font>

```json
{
  "cancelTime": "2026-03-02 09:16:32",
  "orderNo": "A202603020916285367",
  "orderStatus": 0,
  "remark": "",
  "thirdOrderNo": "ORD20260302091628944002216"
}
```

+ <font style="color:rgb(49, 49, 49);">Successful response</font>

```json
{
    "code": 200,
    "msg": "取消訂單成功",
    "data": {
        "orderNo": "A202603020916285367",
        "thirdOrderNo": "ORD20260302091628944002216",
        "returnCode": "success",
        "returnMsg": ""
    }
}
```
