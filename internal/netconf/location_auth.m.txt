// location-auth.m — one-shot Location authorization helper for netcfg on
// macOS 13+.
//
// macOS treats Wi-Fi SSIDs as location information and redacts them from
// system_profiler / airport / networksetup unless the *terminal app* running
// the command has Location access. Terminal never appears in System Settings
// > Privacy & Security > Location Services on its own because it never asks;
// this helper asks exactly once on its behalf.
//
// Build and run it from the SAME terminal app you later run netcfg in:
//
//   clang -framework Foundation -framework CoreLocation \
//       scripts/location-auth.m -o /tmp/location-auth
//   /tmp/location-auth
//
// Answer "Allow" (允许) in the system prompt. The terminal app then shows up
// in Location Services and `sudo netcfg serve` can list Wi-Fi networks. The
// binary is not needed afterwards.
#import <Foundation/Foundation.h>
#import <CoreLocation/CoreLocation.h>

@interface AuthDelegate : NSObject <CLLocationManagerDelegate>
@end

@implementation AuthDelegate
- (void)locationManagerDidChangeAuthorization:(CLLocationManager *)manager {
    switch (manager.authorizationStatus) {
        case kCLAuthorizationStatusAuthorized:
            fprintf(stdout, "location access granted; SSIDs should now be visible to netcfg\n");
            exit(0);
        case kCLAuthorizationStatusDenied:
        case kCLAuthorizationStatusRestricted:
            fprintf(stdout, "location access denied; SSIDs stay hidden\n");
            exit(1);
        default:
            break; // notDetermined yet — keep waiting for the user
    }
}
@end

int main(void) {
    @autoreleasepool {
        CLLocationManager *manager = [[CLLocationManager alloc] init];
        AuthDelegate *delegate = [[AuthDelegate alloc] init];
        manager.delegate = delegate;
        [manager requestWhenInUseAuthorization];
        fprintf(stdout, "waiting for the system prompt...\n");
        [[NSRunLoop mainRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:90]];
        fprintf(stdout, "timed out; no prompt was answered\n");
    }
    return 1;
}
