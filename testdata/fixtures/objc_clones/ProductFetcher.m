#import "ProductFetcher.h"

@implementation ProductFetcher

- (void)fetchProductWithID:(NSString *)productID completion:(void (^)(NSDictionary *, NSError *))completion {
    NSString *urlString = [NSString stringWithFormat:@"https://api.example.com/products/%@", productID];
    NSURL *url = [NSURL URLWithString:urlString];
    NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:url];
    [request setHTTPMethod:@"GET"];
    [request setValue:@"application/json" forHTTPHeaderField:@"Content-Type"];
    [request setValue:self.authToken forHTTPHeaderField:@"Authorization"];

    NSURLSessionDataTask *task = [[NSURLSession sharedSession] dataTaskWithRequest:request
        completionHandler:^(NSData *data, NSURLResponse *response, NSError *error) {
            if (error) {
                NSLog(@"Error fetching product: %@", error);
                completion(nil, error);
                return;
            }

            NSHTTPURLResponse *httpResponse = (NSHTTPURLResponse *)response;
            if (httpResponse.statusCode != 200) {
                NSError *statusError = [NSError errorWithDomain:@"APIError"
                                                          code:httpResponse.statusCode
                                                      userInfo:@{NSLocalizedDescriptionKey: @"Product fetch failed"}];
                completion(nil, statusError);
                return;
            }

            NSError *parseError;
            NSDictionary *json = [NSJSONSerialization JSONObjectWithData:data options:0 error:&parseError];
            if (parseError) {
                NSLog(@"Error parsing product response: %@", parseError);
                completion(nil, parseError);
                return;
            }

            completion(json, nil);
        }];
    [task resume];
}

@end
